package msauth

import (
	"context"
	"errors"
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

func TestPollForTokenReportsExpiredCode(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		inFlight bool // hold the poll request open until the test ends
	}{
		{
			name: "deadline passes while authorization is pending",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"authorization_pending"}`)
			},
		},
		{
			name:     "deadline passes while a poll request is in flight",
			inFlight: true,
		},
		{
			name: "server reports expired_token",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"expired_token","error_description":"code expired"}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			release := make(chan struct{})
			if tt.inFlight {
				handler = func(_ http.ResponseWriter, r *http.Request) {
					select {
					case <-r.Context().Done():
					case <-release:
					}
				}
			}
			srv := httptest.NewServer(handler)
			defer srv.Close()
			defer close(release) // runs before srv.Close so a held request can finish
			overrideAuthority(t, srv.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			// expiresIn of 2s lets at least one poll start before the deadline.
			_, err := PollForToken(ctx, testClientID, testTenantID, "device-code", 1, 2, "verifier", false)
			if !errors.Is(err, ErrDeviceCodeExpired) {
				t.Fatalf("error = %v, want ErrDeviceCodeExpired", err)
			}
			if strings.Contains(err.Error(), "deadline exceeded") {
				t.Errorf("error %q still leaks the raw context error", err)
			}
		})
	}
}

func TestPollForTokenCancellationIsNotExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"authorization_pending"}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(1500*time.Millisecond, cancel)
	_, err := PollForToken(ctx, testClientID, testTenantID, "device-code", 1, 60, "verifier", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrDeviceCodeExpired) {
		t.Fatal("a cancelled login must not be reported as an expired code")
	}
}
