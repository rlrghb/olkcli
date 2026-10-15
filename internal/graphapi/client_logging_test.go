package graphapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type pipelineFunc func(*http.Request) (*http.Response, error)

func (f pipelineFunc) Next(req *http.Request, _ int) (*http.Response, error) { return f(req) }

// failingBody yields data, then fails as a dropped connection would.
type failingBody struct{ r io.Reader }

var errDropped = errors.New("connection dropped")

func (b *failingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == io.EOF {
		return n, errDropped
	}
	return n, err
}
func (b *failingBody) Close() error { return nil }

// logged runs one response through the logger, returning it and the log. The
// caller reads the returned body; tests close nothing real.
func logged(t *testing.T, status int, body io.ReadCloser) (resp *http.Response, logText string) {
	t.Helper()
	var log bytes.Buffer
	m := &loggingMiddleware{out: &log}
	req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)
	resp, err := m.Intercept(pipelineFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: http.Header{}, Body: body, Request: r}, nil
	}), 0, req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, log.String()
}

// Under --verbose a download cut off mid-body must still fail, not end cleanly
// with what arrived.
func TestVerboseKeepsBodyReadErrors(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		resp, _ := logged(t, status, &failingBody{strings.NewReader("partial")})
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if !errors.Is(err, errDropped) {
			t.Errorf("HTTP %d: read = %q, %v; want the dropped-connection error", status, data, err)
		}
	}
}

func TestVerboseDumpsOnlyErrorBodiesAndKeepsThemWhole(t *testing.T) {
	resp, log := logged(t, http.StatusOK, io.NopCloser(strings.NewReader("secret-file-bytes")))
	defer resp.Body.Close()
	if strings.Contains(log, "secret-file-bytes") {
		t.Errorf("a successful body was logged: %s", log)
	}
	if data, _ := io.ReadAll(resp.Body); string(data) != "secret-file-bytes" {
		t.Errorf("successful body = %q", data)
	}

	long := strings.Repeat("e", verboseBodyLimit+10)
	resp, log = logged(t, http.StatusBadRequest, io.NopCloser(strings.NewReader(long)))
	defer resp.Body.Close()
	if !strings.Contains(log, "…(truncated)") {
		t.Error("an oversized error body was not marked truncated")
	}
	if data, _ := io.ReadAll(resp.Body); string(data) != long {
		t.Errorf("error body came back %d bytes, want %d", len(data), len(long))
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

// The retry handler drops a 429 or 5xx response without closing it, so the
// logger closes every error body it reads.
func TestVerboseClosesErrorBodies(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader(strings.Repeat("e", verboseBodyLimit*2))}
	resp, _ := logged(t, http.StatusServiceUnavailable, body)
	defer resp.Body.Close()
	if !body.closed {
		t.Error("the error body's connection was left open")
	}
	if data, _ := io.ReadAll(resp.Body); len(data) != verboseBodyLimit*2 {
		t.Errorf("error body came back %d bytes", len(data))
	}
}

// A request to a pre-authenticated download or upload URL is logged without
// its path or query, which carry the credential; Graph's own are logged whole.
func TestVerboseRedactsPreauthenticatedURLs(t *testing.T) {
	var log bytes.Buffer
	m := &loggingMiddleware{out: &log}
	for _, raw := range []string{
		"https://contoso-my.sharepoint.com/personal/x/_layouts/15/download.aspx?tempauth=SECRET-TOKEN",
		"https://graph.microsoft.com/v1.0/me/messages?$top=5",
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, http.NoBody)
		resp, _ := m.Intercept(pipelineFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New(`Get "` + raw + `": connection reset`)
		}), 0, req)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	out := log.String()
	if strings.Contains(out, "SECRET-TOKEN") || strings.Contains(out, "download.aspx") {
		t.Errorf("verbose log leaks a pre-authenticated URL:\n%s", out)
	}
	if !strings.Contains(out, "$top=5") {
		t.Errorf("Graph request URL not logged whole:\n%s", out)
	}
}

// An error body past the cap ends in an error, not a clean EOF.
func TestVerboseOversizedErrorBodyIsNotComplete(t *testing.T) {
	resp, _ := logged(t, http.StatusBadRequest, io.NopCloser(strings.NewReader(strings.Repeat("e", maxErrorBody+10))))
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err == nil || len(data) != maxErrorBody {
		t.Errorf("read %d bytes, err %v; want %d bytes then an error", len(data), err, maxErrorBody)
	}
}

// Bodies and headers from outside Graph, and redirect bodies, can echo a
// pre-authenticated URL, so verbose output withholds them; Graph URLs lose
// user info and credential-like query values.
func TestVerboseWithholdsCredentialCarriers(t *testing.T) {
	cases := []struct {
		url    string
		status int
		header http.Header
		body   string
	}{
		{"https://contoso.sharepoint.com/x?tempauth=SECRET-TOKEN", http.StatusForbidden, nil, `{"error":"bad tempauth=SECRET-TOKEN"}`},
		{"https://graph.microsoft.com/v1.0/me/drive/items/1/content", http.StatusFound, nil, `<a href="https://x.sharepoint.com/?tempauth=SECRET-TOKEN">`},
		{"https://graph.microsoft.com/v1.0/me", http.StatusBadRequest, http.Header{"Content-Location": {"https://x/?sig=SECRET-TOKEN"}, "Link": {"<https://x/?sig=SECRET-TOKEN>"}}, `{"error":{"code":"BadRequest"}}`},
		{"https://user:SECRET-TOKEN@graph.microsoft.com/v1.0/me?access_token=SECRET-TOKEN&$skiptoken=page2", http.StatusBadRequest, nil, `{}`},
		{"https://graph.microsoft.com/v1.0/me?%74oken%ZZ=SECRET-TOKEN#access_token=SECRET-TOKEN", http.StatusBadRequest, nil, `{}`},
	}
	for _, tc := range cases {
		var log bytes.Buffer
		m := &loggingMiddleware{out: &log}
		req, _ := http.NewRequest(http.MethodGet, tc.url, http.NoBody)
		req.Header.Set("Referer", "https://x/?sig=SECRET-TOKEN")
		resp, err := m.Intercept(pipelineFunc(func(r *http.Request) (*http.Response, error) {
			h := tc.header
			if h == nil {
				h = http.Header{}
			}
			return &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Header: h,
				Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
		}), 0, req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if strings.Contains(log.String(), "SECRET-TOKEN") {
			t.Errorf("%s (HTTP %d) leaked a credential:\n%s", tc.url, tc.status, log.String())
		}
	}
	// Graph's own error bodies and OData paging stay visible for diagnosis.
	var log bytes.Buffer
	m := &loggingMiddleware{out: &log}
	req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me/messages?$skiptoken=page2", http.NoBody)
	resp, _ := m.Intercept(pipelineFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Status: "Bad Request", Header: http.Header{"Request-Id": {"rid-1"}},
			Body: io.NopCloser(strings.NewReader(`{"error":{"code":"BadRequest"}}`)), Request: r}, nil
	}), 0, req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	for _, want := range []string{"$skiptoken=page2", "rid-1", `"code":"BadRequest"`} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("verbose log lost %s:\n%s", want, log.String())
		}
	}
}

// Transport errors are logged by kind; their text quotes the URL however it is
// escaped.
func TestVerboseTransportErrorsAreLoggedByKind(t *testing.T) {
	var log bytes.Buffer
	m := &loggingMiddleware{out: &log}
	raw := `https://graph.microsoft.com/v1.0/me?access_token=SECRET-TOKEN&x="q"`
	req, _ := http.NewRequest(http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)
	resp, err := m.Intercept(pipelineFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: raw, Err: errors.New("connection reset")}
	}), 0, req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("error swallowed")
	}
	if strings.Contains(log.String(), "SECRET-TOKEN") {
		t.Errorf("transport error leaked a credential:\n%s", log.String())
	}
}
