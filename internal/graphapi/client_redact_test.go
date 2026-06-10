package graphapi

import "testing"

func TestRedactHeader(t *testing.T) {
	for _, k := range []string{"Authorization", "authorization", "Set-Cookie", "Cookie", "WWW-Authenticate", "Location", "Proxy-Authorization"} {
		if got := redactHeader(k, []string{"secret-value"}); got != "<redacted>" {
			t.Errorf("redactHeader(%q) = %q, want <redacted>", k, got)
		}
	}
	for _, k := range []string{"Content-Type", "X-Request-Id", "Date"} {
		if got := redactHeader(k, []string{"v1", "v2"}); got != "v1, v2" {
			t.Errorf("redactHeader(%q) = %q, want joined values", k, got)
		}
	}
}
