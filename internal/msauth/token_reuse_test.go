package msauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

func TestConcurrentCredentialsReuseStoredAccessToken(t *testing.T) {
	isolateLocks(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"access_token":"access-fixture","refresh_token":"rotated-fixture","expires_in":3600}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)
	store := newLockedStore()
	const email = "reuse@example.com"
	if err := StoreToken(store, email, &TokenData{RefreshToken: "legacy-fixture", Email: email}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			// A fresh authenticator must reload the persisted pair, as another
			// CLI process does, rather than relying on an in-memory token.
			credential, err := NewAuthenticator(store, testClientID, testTenantID).GetCredential(context.Background(), email)
			if err != nil {
				t.Error(err)
				return
			}
			token, err := credential.GetToken(context.Background(), policy.TokenRequestOptions{})
			if err != nil || token.Token != "access-fixture" {
				t.Error("cached credential did not return the issued token")
			}
		})
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	stored, err := LoadToken(store, email)
	if err != nil || stored.RefreshToken != "rotated-fixture" || stored.ExpiresAt.Before(time.Now().Add(50*time.Minute)) {
		t.Fatal("rotated refresh token and access expiry were not retained")
	}
}

func TestCredentialRefreshesMissingOrExpiringAccessToken(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			isolateLocks(t)
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, `{"access_token":"new-fixture","expires_in":3600}`)
			}))
			defer srv.Close()
			overrideAuthority(t, srv.URL)
			data := &TokenData{AccessToken: "old-fixture", RefreshToken: "refresh-fixture", ExpiresAt: time.Now().Add(time.Minute)}
			if missing {
				data.AccessToken = ""
				data.ExpiresAt = time.Now().Add(time.Hour)
			}
			store := newLockedStore()
			if err := StoreToken(store, "expiry@example.com", data); err != nil {
				t.Fatal(err)
			}
			_, err := NewAuthenticator(store, testClientID, testTenantID).GetCredential(context.Background(), "expiry@example.com")
			if err != nil || calls != 1 {
				t.Fatalf("refresh calls = %d, error = %v", calls, err)
			}
			stored, err := LoadToken(store, "expiry@example.com")
			if err != nil || stored.RefreshToken != "refresh-fixture" {
				t.Fatal("omitted rotation lost the existing refresh token")
			}
		})
	}
}

func TestOAuthThrottlingRetainsDiagnosticsAndStoredToken(t *testing.T) {
	isolateLocks(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"temporarily_unavailable","error_description":"AADSTS40012: private-response-fixture AADSTS40012"}`)
	}))
	defer srv.Close()
	overrideAuthority(t, srv.URL)
	store := newLockedStore()
	if err := StoreToken(store, "throttle@example.com", &TokenData{RefreshToken: "keep-fixture"}); err != nil {
		t.Fatal(err)
	}
	_, err := NewAuthenticator(store, testClientID, testTenantID).GetCredential(context.Background(), "throttle@example.com")
	var refresh *RefreshError
	if !errors.As(err, &refresh) || refresh.Status != 429 || refresh.Code != "temporarily_unavailable" || refresh.RetryAfterSeconds != 120 {
		t.Fatalf("missing structured refresh failure: %v", err)
	}
	if len(refresh.AADSTSCodes) != 1 || refresh.AADSTSCodes[0] != 40012 || strings.Contains(err.Error(), "private-response-fixture") {
		t.Fatal("OAuth diagnostics lost the provider code or exposed response text")
	}
	stored, loadErr := LoadToken(store, "throttle@example.com")
	if loadErr != nil || stored.RefreshToken != "keep-fixture" {
		t.Fatal("failed refresh changed stored credentials")
	}
}
