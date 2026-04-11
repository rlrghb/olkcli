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
	"strings"
	"time"
)

var validTenantID = regexp.MustCompile(`^(?:common|organizations|consumers|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
var validClientID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validateTenantID(tenantID string) error {
	if !validTenantID.MatchString(tenantID) {
		return fmt.Errorf("invalid tenant ID %q: must be a UUID or one of: common, organizations, consumers", tenantID)
	}
	return nil
}

func validateClientID(clientID string) error {
	if !validClientID.MatchString(clientID) {
		return fmt.Errorf("invalid client ID %q: must be a UUID", clientID)
	}
	return nil
}

// httpClient is a shared HTTP client with a sensible timeout for auth operations.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// DeviceCodeResponse holds the response from the device code authorization request.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// TokenResponse holds the OAuth2 token response from the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// ErrorResponse represents an OAuth2 error response.
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// deviceCodeURL returns the device code endpoint for the given tenant.
func deviceCodeURL(tenantID string) string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode", tenantID)
}

// tokenURL returns the token endpoint for the given tenant.
func tokenURL(tenantID string) string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
}

// RequestDeviceCode initiates the device code flow by requesting a device code
// from the Microsoft identity platform.
func RequestDeviceCode(ctx context.Context, clientID, tenantID string, scopes []string) (*DeviceCodeResponse, error) {
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	if err := validateTenantID(tenantID); err != nil {
		return nil, err
	}

	data := url.Values{
		"client_id": {clientID},
		"scope":     {strings.Join(scopes, " ")},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL(tenantID), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<10)) // 100 KB limit for OAuth responses
	if err != nil {
		return nil, fmt.Errorf("reading device code response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
			return nil, fmt.Errorf("device code request failed: %s: %s", sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
		}
		return nil, fmt.Errorf("device code request failed with status %d", resp.StatusCode)
	}

	var dcResp DeviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("decoding device code response: %w", err)
	}

	if dcResp.DeviceCode == "" {
		return nil, fmt.Errorf("device code response contained empty device_code")
	}

	// Bounds-check interval: default to 5, cap at 120.
	if dcResp.Interval <= 0 || dcResp.Interval > 120 {
		dcResp.Interval = 5
	}

	// Bounds-check ExpiresIn to prevent time.Duration overflow (cap at 1 hour).
	if dcResp.ExpiresIn <= 0 || dcResp.ExpiresIn > 3600 {
		dcResp.ExpiresIn = 900
	}

	return &dcResp, nil
}

// PollForToken polls the token endpoint until the user completes authentication,
// the device code expires, or an unrecoverable error occurs.
// expiresIn from the device code response caps the maximum polling duration.
func PollForToken(ctx context.Context, clientID, tenantID, deviceCode string, interval int, expiresIn int, verbose bool) (*TokenResponse, error) {
	if err := validateClientID(clientID); err != nil {
		return nil, err
	}
	if err := validateTenantID(tenantID); err != nil {
		return nil, err
	}

	if interval <= 0 || interval > 120 {
		interval = 5
	}

	// Enforce a maximum polling duration based on the device code expiry.
	if expiresIn > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(expiresIn)*time.Second)
		defer cancel()
	}

	data := url.Values{
		"client_id":   {clientID},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL(tenantID), strings.NewReader(data.Encode()))
		if err != nil {
			return nil, fmt.Errorf("creating token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("polling for token: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<10)) // 100 KB limit for OAuth responses
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading token response: %w", err)
		}

		// Check for pending/slow_down errors.
		if resp.StatusCode != http.StatusOK {
			var errResp ErrorResponse
			if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "[verbose] poll response: status=%d error=%s description=%s\n", resp.StatusCode, sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
				}
				switch errResp.Error {
				case "authorization_pending":
					// User hasn't completed auth yet; keep polling.
					continue
				case "slow_down":
					// Server asks us to slow down; increase interval.
					interval += 5
					if interval > 120 {
						interval = 120
					}
					continue
				default:
					return nil, fmt.Errorf("token request failed: %s: %s", sanitizeStr(errResp.Error), sanitizeStr(errResp.ErrorDescription))
				}
			}
			return nil, fmt.Errorf("token request failed with status %d", resp.StatusCode)
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "[verbose] poll response: status=%d (success)\n", resp.StatusCode)
		}

		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, fmt.Errorf("decoding token response: %w", err)
		}
		if tokenResp.AccessToken == "" {
			return nil, fmt.Errorf("token response contained empty access token")
		}

		return &tokenResp, nil
	}
}
