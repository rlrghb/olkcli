package graphapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	az "github.com/microsoft/kiota-authentication-azure-go"
	nethttplibrary "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
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
	validHosts := []string{
		"graph.microsoft.com",
		"graph.microsoft.us",
		"dod-graph.microsoft.us",
		"graph.microsoft.de",
		"microsoftgraph.chinacloudapi.cn",
		"canary.graph.microsoft.com",
	}
	scopes := []string{"https://graph.microsoft.com/.default"}

	auth, err := az.NewAzureIdentityAuthenticationProviderWithScopesAndValidHosts(cred, scopes, validHosts)
	if err != nil {
		return nil, err
	}

	httpClient := nethttplibrary.GetDefaultClient(nethttplibrary.GetDefaultMiddlewares()...)
	// Rewrite /users/me-token-to-replace/ → /me/ so personal Microsoft accounts
	// (MSA/outlook.com) work correctly. The SDK sentinel is only resolved
	// server-side for AAD/Entra tokens; MSA tokens require the literal /me path.
	var transport http.RoundTripper = &meRewriteTransport{wrapped: httpClient.Transport}
	if verbose {
		transport = &loggingTransport{wrapped: transport, out: os.Stderr}
	}
	httpClient.Transport = transport

	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(auth, nil, nil, httpClient)
	if err != nil {
		return nil, err
	}

	inner := msgraphsdk.NewGraphServiceClient(adapter)
	return &Client{inner: inner}, nil
}

// loggingTransport logs HTTP requests and responses (redacting Authorization headers) to out.
type loggingTransport struct {
	wrapped http.RoundTripper
	out     io.Writer
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	w := t.out
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

	resp, err := t.wrapped.RoundTrip(req)
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

// meRewriteTransport rewrites /users/me-token-to-replace/ to /me/ in request
// URLs. The Graph SDK's Me() method uses a sentinel path parameter that the
// Graph service only resolves for AAD/Entra delegated tokens. Personal Microsoft
// accounts (MSA/outlook.com) require the literal /me path instead.
type meRewriteTransport struct {
	wrapped http.RoundTripper
}

func (t *meRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	const sentinel = "/users/me-token-to-replace/"
	if strings.Contains(req.URL.Path, sentinel) {
		// Clone the request to avoid mutating the original.
		reqCopy := req.Clone(req.Context())
		reqCopy.URL.Path = strings.Replace(reqCopy.URL.Path, sentinel, "/me/", 1)
		if reqCopy.URL.RawPath != "" {
			reqCopy.URL.RawPath = strings.Replace(reqCopy.URL.RawPath, sentinel, "/me/", 1)
		}
		req = reqCopy
	}
	return t.wrapped.RoundTrip(req)
}

// Inner returns the underlying Graph SDK client
func (c *Client) Inner() *msgraphsdk.GraphServiceClient {
	return c.inner
}
