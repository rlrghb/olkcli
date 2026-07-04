package msauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	testClientID = "11111111-2222-3333-4444-555555555555"
	testTenantID = "common"
)

func TestGenerateState(t *testing.T) {
	s1, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(s1)
	if err != nil {
		t.Fatalf("state is not base64url: %v", err)
	}
	if len(raw) != 16 {
		t.Errorf("state decodes to %d bytes, want 16", len(raw))
	}
	s2, err := generateState()
	if err != nil {
		t.Fatalf("generateState: %v", err)
	}
	if s1 == s2 {
		t.Error("two generated states are identical")
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	scopes := []string{"offline_access", "User.Read", "Mail.ReadWrite"}
	got := buildAuthorizeURL(testClientID, testTenantID, "http://localhost:12345/callback", "st4te", "ch4llenge", scopes)

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parsing authorize URL: %v", err)
	}
	if u.Host != "login.microsoftonline.com" {
		t.Errorf("host = %q, want login.microsoftonline.com", u.Host)
	}
	if u.Path != "/common/oauth2/v2.0/authorize" {
		t.Errorf("path = %q, want /common/oauth2/v2.0/authorize", u.Path)
	}

	q := u.Query()
	want := map[string]string{
		"client_id":             testClientID,
		"response_type":         "code",
		"redirect_uri":          "http://localhost:12345/callback",
		"scope":                 "offline_access User.Read Mail.ReadWrite",
		"state":                 "st4te",
		"code_challenge":        "ch4llenge",
		"code_challenge_method": "S256",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), v)
		}
	}
	for _, absent := range []string{"response_mode", "prompt", "nonce"} {
		if q.Has(absent) {
			t.Errorf("query unexpectedly contains %s", absent)
		}
	}
}

// callbackGet routes a GET through the handler and returns the recorder.
func callbackGet(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCallbackHandlerSuccess(t *testing.T) {
	ch := make(chan authCodeResult, 1)
	h := newCallbackHandler("good-state", ch)

	w := callbackGet(t, h, "/callback?code=abc123&state=good-state")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if res.code != "abc123" {
			t.Errorf("code = %q, want abc123", res.code)
		}
	default:
		t.Fatal("no result delivered")
	}
}

func TestCallbackHandlerStateMismatch(t *testing.T) {
	ch := make(chan authCodeResult, 1)
	h := newCallbackHandler("good-state", ch)

	for _, target := range []string{
		"/callback?code=abc123&state=evil-state",
		"/callback?code=abc123",
		"/callback?error=access_denied&error_description=forged",
	} {
		w := callbackGet(t, h, target)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, w.Code)
		}
		select {
		case res := <-ch:
			t.Errorf("%s: flow consumed by invalid request: %+v", target, res)
		default:
		}
	}

	// The pending login must still be completable after invalid hits.
	callbackGet(t, h, "/callback?code=real&state=good-state")
	select {
	case res := <-ch:
		if res.code != "real" {
			t.Errorf("code = %q, want real", res.code)
		}
	default:
		t.Fatal("valid callback after invalid hits was not delivered")
	}
}

func TestCallbackHandlerAccessDenied(t *testing.T) {
	ch := make(chan authCodeResult, 1)
	h := newCallbackHandler("good-state", ch)

	desc := "user declined \x1b[31mconsent"
	w := callbackGet(t, h, "/callback?error=access_denied&error_description="+url.QueryEscape(desc)+"&state=good-state")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "access_denied") || strings.Contains(w.Body.String(), "declined") {
		t.Error("server-provided strings leaked into the HTML response")
	}
	select {
	case res := <-ch:
		if res.err == nil {
			t.Fatal("expected error result")
		}
		if !strings.Contains(res.err.Error(), "access_denied") {
			t.Errorf("error %q does not mention access_denied", res.err)
		}
		if strings.Contains(res.err.Error(), "\x1b") {
			t.Error("error contains unsanitized escape sequence")
		}
	default:
		t.Fatal("denial was not delivered")
	}
}

func TestCallbackHandlerWrongPath(t *testing.T) {
	ch := make(chan authCodeResult, 1)
	h := newCallbackHandler("good-state", ch)

	w := callbackGet(t, h, "/favicon.ico")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	select {
	case res := <-ch:
		t.Errorf("flow consumed by wrong-path request: %+v", res)
	default:
	}
}

func TestCallbackHandlerSingleUse(t *testing.T) {
	ch := make(chan authCodeResult, 1)
	h := newCallbackHandler("good-state", ch)

	callbackGet(t, h, "/callback?code=first&state=good-state")
	// Drain, simulating the orchestrator receiving the result. A bare
	// buffered channel would accept a second send after this.
	res := <-ch
	if res.code != "first" {
		t.Fatalf("code = %q, want first", res.code)
	}

	w := callbackGet(t, h, "/callback?code=second&state=good-state")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "already completed") {
		t.Error("replayed callback did not get the already-completed page")
	}
	select {
	case res := <-ch:
		t.Errorf("replayed callback delivered a second result: %+v", res)
	default:
	}
}

// overrideAuthority points authorityBase at a test server for the duration of
// the test. Tests using it must not run in parallel.
func overrideAuthority(t *testing.T, base string) {
	t.Helper()
	prev := authorityBase
	authorityBase = base
	t.Cleanup(func() { authorityBase = prev })
}

func TestExchangeCode(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing form: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"at","refresh_token":"rt","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)

	resp, err := exchangeCode(context.Background(), testClientID, testTenantID,
		"the-code", "http://127.0.0.1:9/callback", "the-verifier", []string{"offline_access", "User.Read"})
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if resp.AccessToken != "at" || resp.RefreshToken != "rt" {
		t.Errorf("unexpected token response: %+v", resp)
	}

	want := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     testClientID,
		"code":          "the-code",
		"redirect_uri":  "http://127.0.0.1:9/callback",
		"code_verifier": "the-verifier",
		"scope":         "offline_access User.Read",
	}
	for k, v := range want {
		if gotForm.Get(k) != v {
			t.Errorf("form %s = %q, want %q", k, gotForm.Get(k), v)
		}
	}
	if gotForm.Has("client_secret") {
		t.Error("public client must not send client_secret")
	}
}

func TestExchangeCodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"AADSTS70008: expired"}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)

	_, err := exchangeCode(context.Background(), testClientID, testTenantID,
		"bad-code", "http://127.0.0.1:9/callback", "v", []string{"User.Read"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") || !strings.Contains(err.Error(), "AADSTS70008") {
		t.Errorf("error %q missing OAuth error details", err)
	}
}

// memStore is an in-memory secrets.Store for tests.
type memStore struct {
	m map[string]string
}

func newMemStore() *memStore { return &memStore{m: make(map[string]string)} }

func (s *memStore) Set(key, value string) error { s.m[key] = value; return nil }

func (s *memStore) Get(key string) (string, error) {
	v, ok := s.m[key]
	if !ok {
		return "", fmt.Errorf("key %q not found", key)
	}
	return v, nil
}

func (s *memStore) Delete(key string) error { delete(s.m, key); return nil }

func (s *memStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(s.m))
	for k := range s.m {
		keys = append(keys, k)
	}
	return keys, nil
}

// overrideBrowser stubs openBrowser for the duration of the test.
func overrideBrowser(t *testing.T, fn func(string) error) {
	t.Helper()
	prev := openBrowser
	openBrowser = fn
	t.Cleanup(func() { openBrowser = prev })
}

// startFakeIdP runs a test server that acts as both the token endpoint and
// the Graph /me endpoint, wiring authorityBase and graphMeURL to it.
func startFakeIdP(t *testing.T, tokenJSON string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, tokenJSON)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/me") {
			fmt.Fprint(w, `{"mail":"user@example.com","displayName":"Test User"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	overrideAuthority(t, srv.URL)

	prevMe := graphMeURL
	graphMeURL = srv.URL + "/me"
	t.Cleanup(func() { graphMeURL = prevMe })
}

// simulateRedirect returns a browser stub that immediately performs the IdP
// redirect: it parses the authorize URL and GETs the loopback callback with
// the given extra query parameters (plus the request's own state).
func simulateRedirect(t *testing.T, extra url.Values) func(string) error {
	t.Helper()
	return func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		cb, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			return err
		}
		cbq := url.Values{"state": {q.Get("state")}}
		for k, vs := range extra {
			cbq[k] = vs
		}
		cb.RawQuery = cbq.Encode()
		go func() {
			resp, err := http.Get(cb.String())
			if err != nil {
				t.Errorf("callback GET: %v", err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}
}

func TestLoginAuthCodeEndToEnd(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", t.TempDir())
	startFakeIdP(t, `{"access_token":"at","refresh_token":"rt","expires_in":3600,"token_type":"Bearer"}`)
	overrideBrowser(t, simulateRedirect(t, url.Values{"code": {"fake-code"}}))

	store := newMemStore()
	auth := NewAuthenticator(store, testClientID, testTenantID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := auth.LoginAuthCode(ctx, []string{"offline_access", "User.Read"}, false)
	if err != nil {
		t.Fatalf("LoginAuthCode: %v", err)
	}
	if info.Email != "user@example.com" || info.DisplayName != "Test User" {
		t.Errorf("unexpected account info: %+v", info)
	}

	raw, err := store.Get("olk:token:user@example.com")
	if err != nil {
		t.Fatalf("token not stored: %v", err)
	}
	var data TokenData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("stored token is not JSON: %v", err)
	}
	if data.RefreshToken != "rt" {
		t.Errorf("stored refresh token = %q, want rt", data.RefreshToken)
	}
	if data.AccessToken != "" {
		t.Error("access token must not be persisted")
	}
}

func TestLoginAuthCodeAccessDenied(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", t.TempDir())
	startFakeIdP(t, `{}`)
	overrideBrowser(t, simulateRedirect(t, url.Values{
		"error":             {"access_denied"},
		"error_description": {"user said no"},
	}))

	auth := NewAuthenticator(newMemStore(), testClientID, testTenantID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := auth.LoginAuthCode(ctx, []string{"User.Read"}, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error %q does not mention access_denied", err)
	}
}

func TestLoginAuthCodeTimeout(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", t.TempDir())
	startFakeIdP(t, `{}`)
	overrideBrowser(t, func(string) error { return nil }) // browser never redirects

	auth := NewAuthenticator(newMemStore(), testClientID, testTenantID)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := auth.LoginAuthCode(ctx, []string{"User.Read"}, false)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "AADSTS50011") {
		t.Errorf("timeout error %q does not mention AADSTS50011 hint", err)
	}
}

func TestLoginAuthCodeNoRefreshToken(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", t.TempDir())
	startFakeIdP(t, `{"access_token":"at","expires_in":3600,"token_type":"Bearer"}`)
	overrideBrowser(t, simulateRedirect(t, url.Values{"code": {"fake-code"}}))

	auth := NewAuthenticator(newMemStore(), testClientID, testTenantID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := auth.LoginAuthCode(ctx, []string{"User.Read"}, false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "offline_access") {
		t.Errorf("error %q does not mention offline_access", err)
	}
}
