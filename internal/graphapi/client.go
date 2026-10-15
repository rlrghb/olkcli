package graphapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	khttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	graphauth "github.com/microsoftgraph/msgraph-sdk-go-core/authentication"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// Client wraps the Graph SDK client
type Client struct {
	inner        *msgraphsdk.GraphServiceClient
	immutableIDs bool

	// Capability guards. When set, mutating methods refuse to run. They are the
	// single enforcement point for --no-write / --no-send, so the guarantee
	// holds across every entry path (CLI, MCP, scripts, delegated --mailbox).
	noWrite bool
	noSend  bool
}

// SetGuards configures the no-write / no-send capability guards on the client.
func (c *Client) SetGuards(noWrite, noSend bool) {
	c.noWrite = noWrite
	c.noSend = noSend
}

// SetImmutableIDs opts Outlook item requests into IDs that remain stable
// across moves within the same mailbox.
func (c *Client) SetImmutableIDs(enabled bool) {
	c.immutableIDs = enabled
}

// ErrNoWrite and ErrNoSend are returned by mutating methods when the
// corresponding guard is active.
var (
	ErrNoWrite = errors.New("refused: writes are disabled (--no-write / OLK_NO_WRITE)")
	ErrNoSend  = errors.New("refused: sending is disabled (--no-send / OLK_NO_SEND)")
)

// ensureWritable returns ErrNoWrite when the no-write guard is active. Call it
// at the top of every method that mutates server state.
func (c *Client) ensureWritable() error {
	if c.noWrite {
		return ErrNoWrite
	}
	return nil
}

// ensureMaySend returns an error when sending is disallowed. Sending implies a
// mutation, so it also honors the no-write guard.
func (c *Client) ensureMaySend() error {
	if c.noWrite {
		return ErrNoWrite
	}
	if c.noSend {
		return ErrNoSend
	}
	return nil
}

// NewClient creates a new Graph API client from a token credential
func NewClient(cred azcore.TokenCredential) (*Client, error) {
	return newClient(cred, false)
}

// NewClientVerbose creates a new Graph API client with HTTP request/response logging to stderr
func NewClientVerbose(cred azcore.TokenCredential) (*Client, error) {
	return newClient(cred, true)
}

func newClient(cred azcore.TokenCredential, verbose bool) (*Client, error) {
	if !verbose {
		inner, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, nil)
		if err != nil {
			return nil, err
		}
		return &Client{inner: inner}, nil
	}

	// Verbose mode rebuilds the client with the SDK's own Graph middleware
	// pipeline — telemetry plus the built-in /users/me-token-to-replace → /me URL
	// rewrite (msgraph-sdk-go-core graph_client_factory.go ReplacementPairs) — and
	// appends a logging handler as the innermost middleware so it observes the
	// final, post-rewrite request. The /me rewrite is part of the default pipeline,
	// so personal Microsoft accounts (MSA/outlook.com) need no special handling;
	// passing nil scopes/hosts to the core auth provider applies the same Graph
	// defaults the standard constructor uses.
	auth, err := graphauth.NewAzureIdentityAuthenticationProviderWithScopesAndValidHosts(cred, nil, nil)
	if err != nil {
		return nil, err
	}

	opts := msgraphsdk.GetDefaultClientOptions()
	middlewares := append(graphcore.GetDefaultMiddlewaresWithOptions(&opts), &loggingMiddleware{out: os.Stderr})
	httpClient := graphcore.GetDefaultClient(&opts, middlewares...)

	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(auth, nil, nil, httpClient)
	if err != nil {
		return nil, err
	}
	return &Client{inner: msgraphsdk.NewGraphServiceClient(adapter)}, nil
}

// loggingMiddleware logs HTTP requests and responses (redacting Authorization
// headers) to out. It runs as the innermost middleware so it sees the request as
// it goes on the wire, after URL rewriting and telemetry handlers have applied.
type loggingMiddleware struct {
	out io.Writer
}

func (m *loggingMiddleware) Intercept(pipeline khttp.Pipeline, middlewareIndex int, req *http.Request) (*http.Response, error) {
	w := m.out
	if w == nil {
		w = os.Stderr
	}

	// Log request
	fmt.Fprintf(w, "[verbose] --> %s %s\n", req.Method, logURL(req.URL))
	for k, vs := range req.Header {
		fmt.Fprintf(w, "[verbose]     %s: %s\n", k, redactHeader(k, vs))
	}

	resp, err := pipeline.Next(req, middlewareIndex)
	if err != nil {
		// Go's transport errors quote the URL, so only their kind is logged.
		fmt.Fprintf(w, "[verbose] <-- error: %s\n", errorKind(err))
		return resp, err
	}

	fmt.Fprintf(w, "[verbose] <-- %s\n", resp.Status)
	for k, vs := range resp.Header {
		fmt.Fprintf(w, "[verbose]     %s: %s\n", k, redactHeader(k, vs))
	}

	// Only a non-2xx body is dumped, so only one is read here. It is read in
	// full, up to maxErrorBody, and its connection closed: the retry handler
	// outside this one drops a 429 or 5xx without closing it. A read error is
	// kept for the caller. A successful body, such as a file download, passes
	// through untouched, keeping both its streaming and any error that ends it.
	if resp.Body != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody+1))
		resp.Body.Close()
		if len(body) > maxErrorBody {
			body = body[:maxErrorBody]
			if readErr == nil {
				readErr = fmt.Errorf("error response body exceeds %d bytes", maxErrorBody)
			}
		}
		// Only Graph's own error bodies are printed. A redirect's body can
		// spell out its pre-authenticated destination, and another host's
		// error can echo the token in its URL.
		shown, suffix := body, ""
		if len(shown) > verboseBodyLimit {
			shown, suffix = shown[:verboseBodyLimit], " …(truncated)"
		}
		switch {
		case !graphHost(req.URL) || (resp.StatusCode >= 300 && resp.StatusCode < 400):
			fmt.Fprintf(w, "[verbose]     body: (withheld: %d bytes)\n", len(body))
		case readErr != nil:
			fmt.Fprintf(w, "[verbose]     body: (read failed after %d bytes)\n", len(body))
		default:
			fmt.Fprintf(w, "[verbose]     body: %s%s\n", string(shown), suffix)
		}
		var rest io.Reader = bytes.NewReader(nil)
		if readErr != nil {
			rest = errReader{readErr}
		}
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), rest))
	}

	return resp, nil
}

// logURL is a request URL fit for the verbose log. Any host other than Graph,
// such as the storage a download is redirected to, gets a pre-authenticated
// URL whose query (and sometimes path) is a credential, so only its scheme and
// host are shown. Graph's own are shown without user info, and with any query
// parameter that looks like a credential redacted; OData's $-prefixed ones,
// $skiptoken included, are kept.
func logURL(u *url.URL) string {
	if !graphHost(u) {
		return u.Scheme + "://" + u.Host + "/<redacted>"
	}
	shown := *u
	shown.User = nil
	shown.Fragment, shown.RawFragment = "", ""
	// Redacted in place, keeping the rest of the query as sent: re-encoding it
	// would print $top as %24top.
	pairs := strings.Split(shown.RawQuery, "&")
	for i, pair := range pairs {
		name, _, _ := strings.Cut(pair, "=")
		decoded, err := url.QueryUnescape(name)
		if err != nil || credentialParam(decoded) {
			pairs[i] = name + "=<redacted>"
		}
	}
	shown.RawQuery = strings.Join(pairs, "&")
	return shown.String()
}

func credentialParam(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "$") {
		return false
	}
	for _, s := range []string{"token", "auth", "sig", "key", "secret", "code", "password"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// errorKind names a transport failure from its type alone.
func errorKind(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.As(err, &ne) && ne.Timeout():
		return "timed out"
	}
	cause := errors.Unwrap(err)
	if cause == nil {
		cause = err
	}
	return fmt.Sprintf("%T", cause) + " (detail withheld: it can quote the URL)"
}

func graphHost(u *url.URL) bool {
	return strings.EqualFold(u.Hostname(), "graph.microsoft.com")
}

// maxErrorBody bounds how much of an error response is kept. Graph's error
// bodies are a few kilobytes.
const maxErrorBody = 4 << 20

// errReader returns err once its data, read before it, runs out.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// verboseBodyLimit bounds how much of a response body the verbose dump prints,
// so a large/hostile response can't amplify memory or flood the log.
const verboseBodyLimit = 64 << 10

// loggedHeaders are the headers verbose logs show: ones useful for diagnosis
// that carry no credential. Any other header, including Authorization,
// cookies, Location and any that may hold a URL, shows as <redacted>.
var loggedHeaders = map[string]bool{
	"accept":                         true,
	"cache-control":                  true,
	"client-request-id":              true,
	"consistencylevel":               true,
	"content-length":                 true,
	"content-type":                   true,
	"date":                           true,
	"prefer":                         true,
	"preference-applied":             true,
	"request-id":                     true,
	"retry-after":                    true,
	"sdkversion":                     true,
	"user-agent":                     true,
	"x-ms-ags-diagnostic":            true,
	"x-ms-resource-unit":             true,
	"x-ms-throttle-limit-percentage": true,
	"x-request-id":                   true,
}

// redactHeader returns the printable value for a header: as sent for a
// header in loggedHeaders, <redacted> otherwise.
func redactHeader(key string, values []string) string {
	if !loggedHeaders[strings.ToLower(key)] {
		return "<redacted>"
	}
	return strings.Join(values, ", ")
}

// Inner returns the underlying Graph SDK client
func (c *Client) Inner() *msgraphsdk.GraphServiceClient {
	return c.inner
}

// targetUser returns the user-scoped request builder for the target mailbox, or
// the signed-in user's mailbox when target is empty. Both Me() and
// Users().ByUserId() return the same *UserItemRequestBuilder type because the
// kiota-generated SDK reuses item-level builders across the /me and /users/{id}
// aliases — so the same chained calls work for either scope. Callers pass an
// empty target to keep the existing /me behavior.
func (c *Client) targetUser(target string) *users.UserItemRequestBuilder {
	if target == "" {
		return c.inner.Me()
	}
	return c.inner.Users().ByUserId(target)
}
