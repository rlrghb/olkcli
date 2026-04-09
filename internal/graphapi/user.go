package graphapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// UserProfile is a simplified user profile for output
type UserProfile struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"mail"`
	UPN         string `json:"userPrincipalName"`
	JobTitle    string `json:"jobTitle,omitempty"`
	Department  string `json:"department,omitempty"`
	Office      string `json:"officeLocation,omitempty"`
	Phone       string `json:"businessPhones,omitempty"`
}

// GetProfile retrieves the current user's profile information
func (c *Client) GetProfile(ctx context.Context) (*UserProfile, error) {
	selectFields := []string{
		"displayName",
		"mail",
		"userPrincipalName",
		"jobTitle",
		"department",
		"officeLocation",
		"businessPhones",
	}

	resp, err := c.inner.Me().Get(ctx, &users.UserItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UserItemRequestBuilderGetQueryParameters{
			Select: selectFields,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting user profile: %w", err)
	}

	profile := &UserProfile{}
	if resp.GetDisplayName() != nil {
		profile.DisplayName = *resp.GetDisplayName()
	}
	if resp.GetMail() != nil {
		profile.Email = *resp.GetMail()
	}
	if resp.GetUserPrincipalName() != nil {
		profile.UPN = *resp.GetUserPrincipalName()
	}
	if resp.GetJobTitle() != nil {
		profile.JobTitle = *resp.GetJobTitle()
	}
	if resp.GetDepartment() != nil {
		profile.Department = *resp.GetDepartment()
	}
	if resp.GetOfficeLocation() != nil {
		profile.Office = *resp.GetOfficeLocation()
	}
	if phones := resp.GetBusinessPhones(); len(phones) > 0 {
		profile.Phone = strings.Join(phones, ", ")
	}

	return profile, nil
}
