package msauth

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// StaticTokenCredential implements azcore.TokenCredential using a pre-obtained
// access token. This bridges our OAuth2 device-code flow tokens into the
// Azure SDK credential system.
type StaticTokenCredential struct {
	accessToken string
	expiresOn   time.Time
}

// NewStaticTokenCredential creates a new StaticTokenCredential with the given
// access token and expiration time.
func NewStaticTokenCredential(token string, expiresOn time.Time) *StaticTokenCredential {
	return &StaticTokenCredential{
		accessToken: token,
		expiresOn:   expiresOn,
	}
}

// GetToken returns the static access token. It satisfies the
// azcore.TokenCredential interface.
func (c *StaticTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{
		Token:     c.accessToken,
		ExpiresOn: c.expiresOn,
	}, nil
}

// Clear overwrites the access token to reduce exposure in memory.
// Note: Go strings are immutable, so this replaces the reference but the
// original bytes may persist until garbage collected. This is a best-effort
// mitigation — use short-lived credentials where possible.
func (c *StaticTokenCredential) Clear() {
	c.accessToken = ""
	c.expiresOn = time.Time{}
}
