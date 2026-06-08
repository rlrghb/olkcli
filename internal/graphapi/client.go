package graphapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
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
	inner *msgraphsdk.GraphServiceClient
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
	fmt.Fprintf(w, "[verbose] --> %s %s\n", req.Method, req.URL.String())
	for k, vs := range req.Header {
		if strings.EqualFold(k, "authorization") {
			fmt.Fprintf(w, "[verbose]     %s: <redacted>\n", k)
			continue
		}
		fmt.Fprintf(w, "[verbose]     %s: %s\n", k, strings.Join(vs, ", "))
	}

	resp, err := pipeline.Next(req, middlewareIndex)
	if err != nil {
		fmt.Fprintf(w, "[verbose] <-- error: %v\n", err)
		return resp, err
	}

	fmt.Fprintf(w, "[verbose] <-- %s\n", resp.Status)
	for k, vs := range resp.Header {
		fmt.Fprintf(w, "[verbose]     %s: %s\n", k, strings.Join(vs, ", "))
	}

	// Always capture body, but only fully dump it on non-2xx
	if resp.Body != nil {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if readErr == nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
			fmt.Fprintf(w, "[verbose]     body: %s\n", string(body))
		}
	}

	return resp, nil
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
