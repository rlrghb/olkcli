package msauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/rlrghb/olkcli/internal/config"
	"github.com/rlrghb/olkcli/internal/secrets"
)

// refreshMu guards per-email token refresh to prevent concurrent refreshes
// from racing on the same keyring entry.
var (
	refreshMuMap sync.Map // map[string]*sync.Mutex
)

// emailMutex returns a per-email mutex for serializing token refresh operations.
func emailMutex(email string) *sync.Mutex {
	val, _ := refreshMuMap.LoadOrStore(email, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// safeExpiresIn bounds-checks an expires_in value (seconds) to prevent
// time.Duration overflow. Returns a sane default (3600) for out-of-range values.
func safeExpiresIn(expiresIn int) int {
	if expiresIn <= 0 || expiresIn > 86400 {
		return 3600
	}
	return expiresIn
}

// sanitizeStr strips control characters from a string to prevent terminal injection.
func sanitizeStr(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// AccountInfo holds the metadata for a logged-in Microsoft account.
type AccountInfo struct {
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	TenantID    string    `json:"tenant_id"`
	ClientID    string    `json:"client_id"`
	LoginTime   time.Time `json:"login_time"`
}

// Authenticator manages Microsoft OAuth2 authentication and token lifecycle.
type Authenticator struct {
	Store    secrets.Store
	ClientID string
	TenantID string
}

// NewAuthenticator creates a new Authenticator with the given credential store
// and Azure AD application identifiers.
func NewAuthenticator(store secrets.Store, clientID, tenantID string) *Authenticator {
	return &Authenticator{
		Store:    store,
		ClientID: clientID,
		TenantID: tenantID,
	}
}

// graphMeResponse represents the relevant fields from the Microsoft Graph /me endpoint.
type graphMeResponse struct {
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
}

// LoginDeviceCode performs the device code flow, retrieves the user profile,
// and persists the tokens and account information.
func (a *Authenticator) LoginDeviceCode(ctx context.Context, scopes []string, verbose bool) (*AccountInfo, error) {
	// Step 1: Request a device code.
	dcResp, err := RequestDeviceCode(ctx, a.ClientID, a.TenantID, scopes)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[verbose] device code response: expires_in=%d interval=%d verification_uri=%s\n", dcResp.ExpiresIn, dcResp.Interval, sanitizeStr(dcResp.VerificationURI))
		fmt.Fprintf(os.Stderr, "[verbose] client_id=%s tenant=%s scopes=%s\n", a.ClientID, a.TenantID, strings.Join(scopes, " "))
	}

	// Step 2: Display the user code and verification URL.
	fmt.Fprintf(os.Stderr, "\nTo sign in, open a browser to:\n  %s\n\nEnter the code: %s\n\nWaiting for authentication...\n", sanitizeStr(dcResp.VerificationURI), sanitizeStr(dcResp.UserCode))

	// Step 3: Poll for the token (includes PKCE verifier).
	tokenResp, err := PollForToken(ctx, a.ClientID, a.TenantID, dcResp.DeviceCode, dcResp.Interval, dcResp.ExpiresIn, dcResp.CodeVerifier, verbose)
	if err != nil {
		return nil, fmt.Errorf("polling for token: %w", err)
	}

	// Clear any stray escape sequences that accumulated during polling.
	fmt.Fprint(os.Stderr, "\r\033[K")

	// Step 4: Fetch user profile from Microsoft Graph.
	email, displayName, err := fetchUserProfile(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("fetching user profile: %w", err)
	}

	// Step 5: Store refresh token in keyring.
	expiresAt := time.Now().Add(time.Duration(safeExpiresIn(tokenResp.ExpiresIn)) * time.Second)
	tokenData := &TokenData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Email:        email,
	}
	if err := StoreToken(a.Store, email, tokenData); err != nil {
		return nil, fmt.Errorf("storing token: %w", err)
	}

	// Step 6: Save AccountInfo to disk.
	acctInfo := &AccountInfo{
		Email:       email,
		DisplayName: displayName,
		TenantID:    a.TenantID,
		ClientID:    a.ClientID,
		LoginTime:   time.Now(),
	}
	if err := saveAccountInfo(acctInfo); err != nil {
		return nil, fmt.Errorf("saving account info: %w", err)
	}

	return acctInfo, nil
}

// GetCredential returns an azcore.TokenCredential for the given email account.
// It loads the stored token, refreshes it if expired, and returns a
// StaticTokenCredential suitable for use with the Azure/Microsoft Graph SDKs.
func (a *Authenticator) GetCredential(ctx context.Context, email string) (azcore.TokenCredential, error) {
	// Serialize token refresh per email to prevent concurrent refreshes from
	// racing on the same keyring entry.
	mu := emailMutex(email)
	mu.Lock()
	defer mu.Unlock()

	// Step 1: Load token from keyring.
	tokenData, err := LoadToken(a.Store, email)
	if err != nil {
		return nil, fmt.Errorf("loading token for %s: %w", email, err)
	}

	// Step 2: If expired (or about to expire within 5 minutes), refresh.
	if time.Now().Add(5 * time.Minute).After(tokenData.ExpiresAt) {
		tokenResp, err := RefreshAccessToken(ctx, a.ClientID, a.TenantID, tokenData.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("refreshing token for %s: %w", email, err)
		}

		tokenData.AccessToken = tokenResp.AccessToken
		tokenData.ExpiresAt = time.Now().Add(time.Duration(safeExpiresIn(tokenResp.ExpiresIn)) * time.Second)
		if tokenResp.RefreshToken != "" {
			tokenData.RefreshToken = tokenResp.RefreshToken
		}

		// Step 3: Store updated token.
		if err := StoreToken(a.Store, email, tokenData); err != nil {
			return nil, fmt.Errorf("storing refreshed token for %s: %w", email, err)
		}
	}

	return NewStaticTokenCredential(tokenData.AccessToken, tokenData.ExpiresAt), nil
}

// Logout removes the stored credentials and account file for the given email.
func (a *Authenticator) Logout(email string) error {
	// Delete token from keyring.
	if err := a.Store.Delete(secrets.TokenKey(email)); err != nil {
		return fmt.Errorf("deleting token for %s: %w", email, err)
	}

	// Remove account file.
	acctFile := accountFilePath(email)
	if err := os.Remove(acctFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing account file for %s: %w", email, err)
	}

	return nil
}

// ListAccounts reads all account JSON files from the accounts directory and
// returns the parsed AccountInfo records.
func (a *Authenticator) ListAccounts() ([]AccountInfo, error) {
	acctDir := config.AccountsDir()

	entries, err := os.ReadDir(acctDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading accounts directory: %w", err)
	}

	var accounts []AccountInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(acctDir, entry.Name()))
		if err != nil {
			continue
		}

		var info AccountInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		accounts = append(accounts, info)
	}

	return accounts, nil
}

// fetchUserProfile retrieves the email and display name from the Microsoft
// Graph /me endpoint.
func fetchUserProfile(ctx context.Context, accessToken string) (email, displayName string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/me", http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("creating profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetching profile: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<10)) // 100 KB limit for OAuth responses
	if err != nil {
		return "", "", fmt.Errorf("reading profile response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("profile request failed with status %d", resp.StatusCode)
	}

	var me graphMeResponse
	if err := json.Unmarshal(body, &me); err != nil {
		return "", "", fmt.Errorf("decoding profile response: %w", err)
	}

	// Prefer the mail field; fall back to userPrincipalName.
	email = me.Mail
	if email == "" {
		email = me.UserPrincipalName
	}
	email = strings.ToLower(email)

	return email, me.DisplayName, nil
}

// saveAccountInfo writes the AccountInfo as JSON to the accounts directory.
func saveAccountInfo(info *AccountInfo) error {
	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("ensuring config directory: %w", err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling account info: %w", err)
	}

	path := accountFilePath(info.Email)
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing account file: %w", err)
	}

	return nil
}

// atomicWriteFile writes data to a temp file then renames it to the target,
// preventing corruption from crashes during write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// accountFilePath returns the path to the account JSON file for the given email.
func accountFilePath(email string) string {
	safe := strings.ToLower(email)
	// Use filepath.Base to strip any directory components
	safe = filepath.Base(safe)
	// Additional sanitization: remove any remaining path-unsafe characters
	safe = strings.ReplaceAll(safe, "..", "_")
	return filepath.Join(config.AccountsDir(), safe+".json")
}
