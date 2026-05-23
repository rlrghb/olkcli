package graphapi

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// Client wraps the Graph SDK client
type Client struct {
	inner *msgraphsdk.GraphServiceClient
}

// NewClient creates a new Graph API client from a token credential
func NewClient(cred azcore.TokenCredential) (*Client, error) {
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, nil)
	if err != nil {
		return nil, err
	}
	return &Client{inner: client}, nil
}

// Inner returns the underlying Graph SDK client
func (c *Client) Inner() *msgraphsdk.GraphServiceClient {
	return c.inner
}

// targetUser returns the user-scoped request builder for the target mailbox, or
// the signed-in user's mailbox when target is empty. Both Me() and
// Users().ByUserId() return the same *UserItemRequestBuilder type because the
// kiota-generated SDK reuses item-level builders across the /me and /users/{id}
// aliases — so the same chained calls work for either scope. Callers pass an
// empty target to keep the existing /me behavior.
func (c *Client) targetUser(target string) *users.UserItemRequestBuilder {
	if target == "" {
		return c.inner.Me()
	}
	return c.inner.Users().ByUserId(target)
}
