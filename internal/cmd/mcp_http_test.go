package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestSecurityMiddleware_Token(t *testing.T) {
	const token = "s3cret-token"
	srv := httptest.NewServer(securityMiddleware(token, okHandler()))
	defer srv.Close()

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing token", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"malformed header", "token-without-bearer", http.StatusUnauthorized},
		{"correct token", "Bearer " + token, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestSecurityMiddleware_OriginRejected(t *testing.T) {
	srv := httptest.NewServer(securityMiddleware("", okHandler()))
	defer srv.Close()

	// A non-loopback browser Origin must be rejected (DNS-rebinding guard).
	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	req.Header.Set("Origin", "http://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-loopback origin status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestNormalizeAddr(t *testing.T) {
	cases := []struct {
		in           string
		wantAddr     string
		wantLoopback bool
	}{
		{"8765", "127.0.0.1:8765", true},
		{":8765", "127.0.0.1:8765", true},
		{"127.0.0.1:8765", "127.0.0.1:8765", true},
		{"localhost:8765", "localhost:8765", true},
		{"0.0.0.0:8765", "0.0.0.0:8765", false},
		{"192.168.1.5:8765", "192.168.1.5:8765", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			addr, loopback := normalizeAddr(tc.in)
			if addr != tc.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tc.wantAddr)
			}
			if loopback != tc.wantLoopback {
				t.Errorf("loopback = %v, want %v", loopback, tc.wantLoopback)
			}
		})
	}
}

func TestResolveToken_FilePrecedence(t *testing.T) {
	// With no inline token and no env, OLK_MCP_TOKEN_FILE is read.
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("OLK_MCP_TOKEN_FILE", tokenFile)

	c := &MCPCmd{}
	got, err := c.resolveToken()
	if err != nil {
		t.Fatalf("resolveToken: %v", err)
	}
	if got != "file-token" {
		t.Errorf("token = %q, want %q (trimmed)", got, "file-token")
	}
}

func labeledHandler(label string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(label))
	})
}

func TestKeyRouter_RoutesByKey(t *testing.T) {
	const safeKey, fullKey = "safe-key", "full-key"
	// Empty full handler is nil to prove an unconfigured profile is never invoked.
	router := keyRouter(safeKey, fullKey, labeledHandler("SAFE"), labeledHandler("FULL"))
	srv := httptest.NewServer(router)
	defer srv.Close()

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
		wantBody   string
	}{
		{"safe key routes to safe", "Bearer " + safeKey, http.StatusOK, "SAFE"},
		{"full key routes to full", "Bearer " + fullKey, http.StatusOK, "FULL"},
		{"wrong key", "Bearer nope", http.StatusUnauthorized, ""},
		{"missing key", "", http.StatusUnauthorized, ""},
		{"malformed header", "no-bearer", http.StatusUnauthorized, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if tc.wantBody != "" {
				body, _ := io.ReadAll(resp.Body)
				if string(body) != tc.wantBody {
					t.Errorf("body = %q, want %q", body, tc.wantBody)
				}
			}
		})
	}
}

func TestKeyRouter_EmptyKeyUnreachable(t *testing.T) {
	// Full profile has no key (nil handler). Presenting any token must never
	// invoke the nil handler — wrong/empty tokens are 401, the safe key works.
	router := keyRouter("safe-key", "", labeledHandler("SAFE"), nil)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp := doAuthReq(t, srv.URL, "Bearer some-full-attempt")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unconfigured-profile attempt status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	resp = doAuthReq(t, srv.URL, "Bearer safe-key")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "SAFE" {
		t.Errorf("safe key body = %q, want %q", body, "SAFE")
	}
}

func TestKeyRouter_OriginRejected(t *testing.T) {
	router := keyRouter("safe-key", "full-key", labeledHandler("SAFE"), labeledHandler("FULL"))
	srv := httptest.NewServer(router)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Authorization", "Bearer full-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-loopback origin status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestResolveKeys(t *testing.T) {
	dir := t.TempDir()
	fullFile := filepath.Join(dir, "full")
	if err := os.WriteFile(fullFile, []byte("  full-from-file\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	// safe from inline env, full from its _FILE variant.
	t.Setenv("OLK_MCP_KEY_SAFE", "safe-inline")
	t.Setenv("OLK_MCP_KEY_SAFE_FILE", "")
	t.Setenv("OLK_MCP_KEY_FULL", "")
	t.Setenv("OLK_MCP_KEY_FULL_FILE", fullFile)

	safe, full, err := resolveKeys()
	if err != nil {
		t.Fatalf("resolveKeys: %v", err)
	}
	if safe != "safe-inline" {
		t.Errorf("safe = %q, want %q", safe, "safe-inline")
	}
	if full != "full-from-file" {
		t.Errorf("full = %q, want %q (trimmed)", full, "full-from-file")
	}

	t.Run("missing file errors", func(t *testing.T) {
		t.Setenv("OLK_MCP_KEY_SAFE", "")
		t.Setenv("OLK_MCP_KEY_SAFE_FILE", filepath.Join(dir, "does-not-exist"))
		if _, err := resolveKey("OLK_MCP_KEY_SAFE"); err == nil {
			t.Error("expected error for missing key file")
		}
	})

	t.Run("empty file resolves empty", func(t *testing.T) {
		empty := filepath.Join(dir, "empty")
		if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
			t.Fatalf("write empty file: %v", err)
		}
		t.Setenv("OLK_MCP_KEY_SAFE", "")
		t.Setenv("OLK_MCP_KEY_SAFE_FILE", empty)
		got, err := resolveKey("OLK_MCP_KEY_SAFE")
		if err != nil {
			t.Fatalf("resolveKey: %v", err)
		}
		if got != "" {
			t.Errorf("key = %q, want empty", got)
		}
	})
}

func doAuthReq(t *testing.T, url, auth string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func warningsContain(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestBuildAuthHandler(t *testing.T) {
	t.Run("identical keys error", func(t *testing.T) {
		if _, _, err := buildAuthHandler("safe", "same", "same", ""); err == nil {
			t.Fatal("expected error for identical safe/full keys")
		}
	})

	t.Run("only safe key, full unreachable", func(t *testing.T) {
		h, _, err := buildAuthHandler("safe", "safe-key", "", "")
		if err != nil {
			t.Fatalf("buildAuthHandler: %v", err)
		}
		srv := httptest.NewServer(h)
		defer srv.Close()

		// Safe key passes auth and reaches the MCP handler (not a 401).
		resp := doAuthReq(t, srv.URL, "Bearer safe-key")
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("safe key was unauthorized; expected to reach handler")
		}
		// Any other key is rejected; the full profile is unreachable.
		resp = doAuthReq(t, srv.URL, "Bearer something-else")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("non-safe key status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("legacy token ignored in key mode warns", func(t *testing.T) {
		_, warnings, err := buildAuthHandler("safe", "safe-key", "", "legacy-token")
		if err != nil {
			t.Fatalf("buildAuthHandler: %v", err)
		}
		if !warningsContain(warnings, "ignored") {
			t.Errorf("want legacy-token-ignored warning, got %v", warnings)
		}
	})

	t.Run("profile ignored in key mode warns", func(t *testing.T) {
		_, warnings, err := buildAuthHandler("full", "safe-key", "", "")
		if err != nil {
			t.Fatalf("buildAuthHandler: %v", err)
		}
		if !warningsContain(warnings, "--profile") {
			t.Errorf("want profile-ignored warning, got %v", warnings)
		}
	})

	t.Run("no keys uses legacy single-token path", func(t *testing.T) {
		h, _, err := buildAuthHandler("safe", "", "", "tok")
		if err != nil {
			t.Fatalf("buildAuthHandler: %v", err)
		}
		srv := httptest.NewServer(h)
		defer srv.Close()

		resp := doAuthReq(t, srv.URL, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("no-token status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		resp = doAuthReq(t, srv.URL, "Bearer tok")
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("legacy token was unauthorized; expected to reach handler")
		}
	})
}
