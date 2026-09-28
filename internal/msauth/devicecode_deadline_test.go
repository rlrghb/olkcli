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

// These tests distinguish the caller's timeout from the code's own lifetime.
func TestPollForTokenCallerDeadlineIsNotCodeExpiry(t *testing.T) {
	for _, expiresIn := range []int{60, 0} {
		t.Run(fmt.Sprintf("expiresIn=%d", expiresIn), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			_, err := PollForToken(ctx, testClientID, testTenantID, "fictional-code", 5, expiresIn, "", false)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("caller deadline = %v, want context.DeadlineExceeded; device lifetime is %d seconds", err, expiresIn)
			}
		})
	}
}

func TestPollForTokenCallerDeadlineDuringRequestIsNotCodeExpiry(t *testing.T) {
	for _, flushHeaders := range []bool{false, true} {
		t.Run(fmt.Sprintf("reading-body=%v", flushHeaders), func(t *testing.T) {
			release := make(chan struct{})
			started := make(chan struct{}, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				if flushHeaders {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer srv.Close()
			defer close(release)
			overrideAuthority(t, srv.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			_, err := PollForToken(ctx, testClientID, testTenantID, "fictional-code", 1, 60, "", false)
			select {
			case <-started:
			default:
				t.Fatal("test did not reach the request path")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("caller timeout during request = %v, want context.DeadlineExceeded with an unexpired code", err)
			}
		})
	}
}

func TestPollForTokenDeviceExpiryHasActionableMessage(t *testing.T) {
	_, err := PollForToken(context.Background(), testClientID, testTenantID, "fictional-code", 5, 1, "", false)
	if err == nil || !strings.Contains(err.Error(), "sign-in code expired") || !strings.Contains(err.Error(), "olk auth login") {
		t.Fatalf("device expiry error = %v, want explanation and retry command", err)
	}
}

func TestPollForTokenProviderExpiryHasActionableMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"expired_token"}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)
	_, err := PollForToken(context.Background(), testClientID, testTenantID, "fictional-code", 1, 60, "", false)
	if err == nil || !strings.Contains(err.Error(), "sign-in code expired") || !strings.Contains(err.Error(), "olk auth login") {
		t.Fatalf("provider expiry error = %v, want explanation and retry command", err)
	}
}
