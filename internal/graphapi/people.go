package graphapi

import (
	"context"
	"fmt"
	"regexp"

	abs "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// safePeopleQuery matches alphanumeric, Unicode letters, spaces, @, dots, hyphens, underscores.
// SECURITY: this whitelist prevents KQL injection in people/directory search queries.
var safePeopleQuery = regexp.MustCompile(`^[\p{L}\p{N} @._-]+$`)

// Person is a simplified person for output
type Person struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	JobTitle    string `json:"jobTitle,omitempty"`
	Department  string `json:"department,omitempty"`
	Company     string `json:"companyName,omitempty"`
}

func (c *Client) SearchPeople(ctx context.Context, query string, top int32) ([]Person, error) {
	top = clampTop(top)

	if !safePeopleQuery.MatchString(query) {
		return nil, fmt.Errorf("search query contains invalid characters (only letters, numbers, spaces, @, ., _, - allowed)")
	}

	config := &users.ItemPeopleRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemPeopleRequestBuilderGetQueryParameters{
			Top:    &top,
			Search: &query,
		},
	}

	resp, err := c.inner.Me().People().Get(ctx, config)
	if err != nil {
		return nil, enterpriseError("searching people", err)
	}

	var people []Person
	for _, p := range resp.GetValue() {
		person := Person{
			DisplayName: derefStr(p.GetDisplayName()),
			JobTitle:    derefStr(p.GetJobTitle()),
			Department:  derefStr(p.GetDepartment()),
			Company:     derefStr(p.GetCompanyName()),
		}
		// Get primary email from scored email addresses
		if addrs := p.GetScoredEmailAddresses(); len(addrs) > 0 {
			if addrs[0].GetAddress() != nil {
				person.Email = *addrs[0].GetAddress()
			}
		}
		people = append(people, person)
	}

	// Fall back to directory search if People API returned no results
	if len(people) == 0 {
		dirPeople, dirErr := c.SearchDirectory(ctx, query, top)
		if dirErr == nil && len(dirPeople) > 0 {
			return dirPeople, nil
		}
	}

	return people, nil
}

// SearchDirectory searches the organization directory via /users with $search
func (c *Client) SearchDirectory(ctx context.Context, query string, top int32) ([]Person, error) {
	top = clampTop(top)

	if !safePeopleQuery.MatchString(query) {
		return nil, fmt.Errorf("search query contains invalid characters (only letters, numbers, spaces, @, ., _, - allowed)")
	}

	search := fmt.Sprintf("\"displayName:%s\" OR \"mail:%s\"", query, query)
	config := &users.UsersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
			Top:    &top,
			Search: &search,
			Select: []string{"displayName", "mail", "jobTitle", "department", "companyName"},
		},
		Headers: abs.NewRequestHeaders(),
	}
	config.Headers.Add("ConsistencyLevel", "eventual")

	resp, err := c.inner.Users().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("searching directory: %s", graphErrorMessage(err))
	}

	var people []Person
	for _, u := range resp.GetValue() {
		person := Person{
			DisplayName: derefStr(u.GetDisplayName()),
			Email:       derefStr(u.GetMail()),
			JobTitle:    derefStr(u.GetJobTitle()),
			Department:  derefStr(u.GetDepartment()),
			Company:     derefStr(u.GetCompanyName()),
		}
		people = append(people, person)
	}
	return people, nil
}
