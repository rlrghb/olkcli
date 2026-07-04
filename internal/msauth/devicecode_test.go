package msauth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPollForTokenTerminalErrors(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantHint bool
	}{
		{
			name:     "conditional access block hints at browser flow",
			body:     `{"error":"access_denied","error_description":"AADSTS53003: blocked by Conditional Access"}`,
			wantHint: true,
		},
		{
			name:     "invalid_grant hints at browser flow",
			body:     `{"error":"invalid_grant","error_description":"AADSTS50199: security check required"}`,
			wantHint: true,
		},
		{
			name:     "user declined gets no browser hint",
			body:     `{"error":"authorization_declined","error_description":"user said no"}`,
			wantHint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()
			overrideAuthority(t, srv.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := PollForToken(ctx, testClientID, testTenantID, "device-code", 1, 60, "verifier", false)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := strings.Contains(err.Error(), "--browser"); got != tt.wantHint {
				t.Errorf("error %q: --browser hint present = %v, want %v", err, got, tt.wantHint)
			}
		})
	}
}
