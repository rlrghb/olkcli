package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
