package msauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
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

// RefreshError preserves safe OAuth diagnostics without retaining response bodies.
type RefreshError struct {
	Status            int
	Code              string
	AADSTSCodes       []int
	RetryAfterSeconds int
}

func (e *RefreshError) Error() string {
	return fmt.Sprintf("refresh token failed: %s (HTTP %d; AADSTS codes %v)", e.Code, e.Status, e.AADSTSCodes)
}

var oauthCodePattern = regexp.MustCompile(`^[a-z_]{1,64}$`)
var aadstsPattern = regexp.MustCompile(`\bAADSTS(\d{4,8})\b`)

func refreshError(resp *http.Response, body []byte) *RefreshError {
	err := &RefreshError{Status: resp.StatusCode, Code: "unknown_oauth_error"}
	var value ErrorResponse
	if json.Unmarshal(body, &value) == nil {
		if oauthCodePattern.MatchString(value.Error) {
			err.Code = value.Error
		}
		seen := map[int]bool{}
		for _, match := range aadstsPattern.FindAllStringSubmatch(value.ErrorDescription, -1) {
			code, parseErr := strconv.Atoi(match[1])
			if parseErr == nil && !seen[code] {
				err.AADSTSCodes = append(err.AADSTSCodes, code)
				seen[code] = true
			}
		}
	}
	if seconds, parseErr := strconv.Atoi(resp.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
		err.RetryAfterSeconds = seconds
	} else if until, parseErr := http.ParseTime(resp.Header.Get("Retry-After")); parseErr == nil {
		if remaining := time.Until(until); remaining > 0 {
			err.RetryAfterSeconds = int((remaining + time.Second - 1) / time.Second)
		}
	}
	return err
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
		return nil, refreshError(resp, body)
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
	// Reuse the short-lived access token across CLI processes. The existing
	// credential store protects it alongside the longer-lived refresh token.
	// GetCredential holds the account lock across this load-refresh-store cycle.
	jsonData, err := json.Marshal(data)
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
