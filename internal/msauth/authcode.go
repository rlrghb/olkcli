package msauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Self-contained pages served to the browser after the redirect. No
// server-provided strings are ever interpolated — detailed errors travel only
// through the returned Go error — so there is no injection surface here.
const (
	callbackSuccessHTML = `<!DOCTYPE html><html><head><title>olk</title></head><body>
<h2>Sign-in complete</h2><p>You can close this window and return to the terminal.</p>
</body></html>`
	callbackErrorHTML = `<!DOCTYPE html><html><head><title>olk</title></head><body>
<h2>Sign-in failed</h2><p>Return to the terminal for details.</p>
</body></html>`
	callbackDoneHTML = `<!DOCTYPE html><html><head><title>olk</title></head><body>
<h2>Login already completed</h2><p>You can close this window.</p>
</body></html>`
)

// authCodeResult carries the outcome of the loopback redirect callback.
type authCodeResult struct {
	code string
	err  error
}

// authorizeURL returns the authorization endpoint for the given tenant.
func authorizeURL(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/authorize", authorityBase, tenantID)
}

// generateState creates a random CSRF token for the authorization request:
// 16 random bytes, base64url-encoded.
func generateState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating state parameter: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// buildAuthorizeURL assembles the full /authorize URL for the
// authorization-code + PKCE flow (RFC 6749 §4.1.1, RFC 7636 §4.3).
func buildAuthorizeURL(clientID, tenantID, redirectURI, state, codeChallenge string, scopes []string) string {
	q := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return authorizeURL(tenantID) + "?" + q.Encode()
}

// newCallbackHandler returns the handler for the loopback redirect listener.
// It serves only /callback, validates the state parameter before anything
// else, and delivers exactly one result on resultCh. Requests with a missing
// or mismatched state never touch the channel, so a stray or forged local
// request cannot consume a pending login.
func newCallbackHandler(expectedState string, resultCh chan<- authCodeResult) http.Handler {
	// A buffered channel alone is not single-use: once the orchestrator
	// drains the first result, a replayed callback could send again. The
	// CompareAndSwap makes delivery exactly-once.
	var delivered atomic.Bool

	serve := func(w http.ResponseWriter, status int, page string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprint(w, page)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// State is a per-login random value; only Microsoft's redirect of
		// this specific authorization request can know it. Entra includes it
		// on error redirects too, so genuine denials pass this check.
		if q.Get("state") != expectedState {
			serve(w, http.StatusBadRequest, callbackErrorHTML)
			return
		}

		var res authCodeResult
		switch {
		case q.Get("error") != "":
			res.err = fmt.Errorf("authorization failed: %s: %s",
				sanitizeStr(q.Get("error")), sanitizeStr(q.Get("error_description")))
		case q.Get("code") == "":
			res.err = fmt.Errorf("authorization redirect contained no code")
		default:
			res.code = q.Get("code")
		}

		if !delivered.CompareAndSwap(false, true) {
			serve(w, http.StatusOK, callbackDoneHTML)
			return
		}
		resultCh <- res

		if res.err != nil {
			serve(w, http.StatusOK, callbackErrorHTML)
			return
		}
		serve(w, http.StatusOK, callbackSuccessHTML)
	})
	return mux
}

// exchangeCode redeems an authorization code for tokens
// (RFC 6749 §4.1.3 with the PKCE verifier from RFC 7636 §4.5).
func exchangeCode(ctx context.Context, clientID, tenantID, code, redirectURI, codeVerifier string, scopes []string) (*TokenResponse, error) {
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	if err := validateTenantID(tenantID); err != nil {
		return nil, err
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
		"scope":         {strings.Join(scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL(tenantID), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating code exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<10)) // 100 KB limit for OAuth responses
	if err != nil {
		return nil, fmt.Errorf("reading code exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
			return nil, fmt.Errorf("code exchange failed: %s: %s", sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
		}
		return nil, fmt.Errorf("code exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding code exchange response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("code exchange response contained empty access token")
	}

	return &tokenResp, nil
}

// openBrowser launches the system default browser at the given URL. It is a
// var so tests can stub the browser interaction. The child is reaped in a
// goroutine so no zombie lingers while the login wait (up to 15 minutes) runs.
var openBrowser = func(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case windowsOS:
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// LoginAuthCode performs the OAuth2 authorization-code + PKCE flow via the
// system browser and a loopback redirect listener, then persists the tokens
// and account information. Unlike device code flow, the browser session can
// carry device identity, so this flow works in tenants whose Conditional
// Access policies require a compliant device or block device code flow.
func (a *Authenticator) LoginAuthCode(ctx context.Context, scopes []string, verbose bool) (*AccountInfo, error) {
	if err := validateClientID(a.ClientID); err != nil {
		return nil, err
	}
	if err := validateTenantID(a.TenantID); err != nil {
		return nil, err
	}

	// Defensive deadline in case the caller supplied an unbounded context.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
	}

	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	// The redirect URI uses localhost because Entra documents port-ignored
	// matching for http://localhost only — a registered http://localhost/callback
	// matches any dynamic port, and a fixed port would collide with other
	// local services. The browser may resolve localhost to 127.0.0.1 or ::1,
	// so listen on both loopback families: IPv4 is required, IPv6 on the same
	// port is best-effort (browsers fall back to the other family on
	// connection-refused, but listening on both avoids relying on that).
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting loopback listener for browser login: %w (rerun without --browser to use device code flow)", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln6, err6 := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", port))
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	resultCh := make(chan authCodeResult, 1)
	serveErrCh := make(chan error, 2)
	srv := &http.Server{
		Handler:           newCallbackHandler(state, resultCh),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { serveErrCh <- srv.Serve(ln) }()
	if err6 == nil {
		go func() { serveErrCh <- srv.Serve(ln6) }()
	}
	// Graceful shutdown lets an in-flight success page finish flushing.
	defer func() {
		sdCtx, sdCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer sdCancel()
		_ = srv.Shutdown(sdCtx)
	}()

	authURL := buildAuthorizeURL(a.ClientID, a.TenantID, redirectURI, state, codeChallenge, scopes)

	if verbose {
		ipv6 := "yes"
		if err6 != nil {
			ipv6 = "no"
		}
		fmt.Fprintf(os.Stderr, "[verbose] loopback listener on port %d (ipv6: %s)\n", port, ipv6)
		fmt.Fprintf(os.Stderr, "[verbose] client_id=%s tenant=%s scopes=%s\n", a.ClientID, a.TenantID, strings.Join(scopes, " "))
	}

	fmt.Fprintf(os.Stderr, "\nOpening your browser to sign in. If it does not open, visit:\n  %s\n\nWaiting for authentication...\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open browser: %v\n", err)
	}

	var res authCodeResult
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for browser authentication: %w — if the browser showed error AADSTS50011, the app registration is missing the http://localhost/callback redirect URI (see README \"Custom App Registration\"); or rerun without --browser to use device code flow", ctx.Err())
	case err := <-serveErrCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return nil, fmt.Errorf("loopback callback server failed: %w", err)
		}
		return nil, fmt.Errorf("loopback callback server closed unexpectedly")
	case res = <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
	}

	tokenResp, err := exchangeCode(ctx, a.ClientID, a.TenantID, res.code, redirectURI, codeVerifier, scopes)
	if err != nil {
		return nil, err
	}

	return a.finishLogin(ctx, tokenResp)
}
