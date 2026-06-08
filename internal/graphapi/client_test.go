package graphapi

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubPipeline implements khttp.Pipeline, building a canned response inside
// Next() so the loggingMiddleware can be exercised without a real HTTP round
// trip. The response is constructed here (rather than passed in) so its Body is
// owned end-to-end by the code under test and its single caller.
type stubPipeline struct {
	status int
	body   string
}

func (p *stubPipeline) Next(req *http.Request, middlewareIndex int) (*http.Response, error) {
	return &http.Response{
		StatusCode: p.status,
		Status:     http.StatusText(p.status),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(p.body)),
	}, nil
}

func TestLoggingMiddlewareRedactsAuthorization(t *testing.T) {
	var buf bytes.Buffer
	mw := &loggingMiddleware{out: &buf}

	req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	req.Header.Set("Client-Request-Id", "abc-123")

	got, err := mw.Intercept(&stubPipeline{status: http.StatusOK, body: `{"ok":true}`}, 0, req)
	if err != nil {
		t.Fatalf("Intercept returned error: %v", err)
	}
	defer got.Body.Close()

	out := buf.String()
	if strings.Contains(out, "super-secret-token") {
		t.Fatalf("token leaked into verbose log:\n%s", out)
	}
	if !strings.Contains(out, "<redacted>") {
		t.Fatalf("expected Authorization header to be redacted:\n%s", out)
	}
	if !strings.Contains(out, "abc-123") {
		t.Fatalf("expected non-sensitive headers to be logged:\n%s", out)
	}
}

func TestLoggingMiddlewareBodyDumpOnlyOnError(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantBody bool
	}{
		{"success body not dumped", http.StatusOK, `{"value":"sensitive"}`, false},
		{"error body dumped", http.StatusForbidden, `{"error":"forbidden"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := &loggingMiddleware{out: &buf}
			req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)

			got, err := mw.Intercept(&stubPipeline{status: tc.status, body: tc.body}, 0, req)
			if err != nil {
				t.Fatalf("Intercept returned error: %v", err)
			}
			defer got.Body.Close()

			if strings.Contains(buf.String(), "body:") != tc.wantBody {
				t.Fatalf("body-dump=%v, want %v; log:\n%s", !tc.wantBody, tc.wantBody, buf.String())
			}

			// The body must remain fully readable by downstream consumers.
			read, _ := io.ReadAll(got.Body)
			if string(read) != tc.body {
				t.Fatalf("downstream body = %q, want %q", string(read), tc.body)
			}
		})
	}
}
