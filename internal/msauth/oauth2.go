package msauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rlrghb/olkcli/internal/secrets"
)

// TokenData holds the persisted token information for an account.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Email        string    `json:"email"`
}

// RefreshAccessToken exchanges a refresh token for a new access token.
func RefreshAccessToken(ctx context.Context, clientID, tenantID, refreshToken string, verbose ...bool) (*TokenResponse, error) {
	isVerbose := len(verbose) > 0 && verbose[0]
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	if err := validateTenantID(tenantID); err != nil {
		return nil, err
	}

	data := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL(tenantID), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<10)) // 100 KB limit for OAuth responses
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
			if isVerbose {
				fmt.Fprintf(os.Stderr, "[verbose] refresh failed: status=%d error=%s description=%s\n", resp.StatusCode, sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
			}
			return nil, fmt.Errorf("refresh token failed: %s: %s", sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
		}
		return nil, fmt.Errorf("refresh token failed with status %d", resp.StatusCode)
	}

	if isVerbose {
		fmt.Fprintf(os.Stderr, "[verbose] token refresh successful\n")
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("refresh response contained empty access token")
	}

	return &tokenResp, nil
}

// StoreToken serializes TokenData to JSON and persists it in the keyring.
func StoreToken(store secrets.Store, email string, data *TokenData) error {
	// Only persist the refresh token — access tokens are short-lived
	persist := &TokenData{
		RefreshToken: data.RefreshToken,
		Email:        data.Email,
		// ExpiresAt intentionally zero to trigger refresh on next load
	}
	jsonData, err := json.Marshal(persist)
	if err != nil {
		return fmt.Errorf("marshaling token data: %w", err)
	}
	if err := store.Set(secrets.TokenKey(email), string(jsonData)); err != nil {
		return fmt.Errorf("storing token in keyring: %w", err)
	}
	return nil
}

// LoadToken retrieves and deserializes TokenData from the keyring.
func LoadToken(store secrets.Store, email string) (*TokenData, error) {
	raw, err := store.Get(secrets.TokenKey(email))
	if err != nil {
		return nil, fmt.Errorf("loading token from keyring: %w", err)
	}
	var data TokenData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("unmarshaling token data: %w", err)
	}
	if data.RefreshToken == "" {
		return nil, fmt.Errorf("stored token data contains empty refresh token")
	}
	return &data, nil
}
