package graphapi

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

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

	config := &users.ItemPeopleRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemPeopleRequestBuilderGetQueryParameters{
			Top:    &top,
			Search: &query,
		},
	}

	resp, err := c.inner.Me().People().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("searching people: %s (note: this feature requires a work/school account)", graphErrorMessage(err))
	}

	people := make([]Person, 0, top)
	for _, p := range resp.GetValue() {
		person := Person{
			DisplayName: derefStr(p.GetDisplayName()),
			JobTitle:    derefStr(p.GetJobTitle()),
			Department:  derefStr(p.GetDepartment()),
			Company:     derefStr(p.GetCompanyName()),
		}
		if addrs := p.GetScoredEmailAddresses(); len(addrs) > 0 {
			if addrs[0].GetAddress() != nil {
				person.Email = *addrs[0].GetAddress()
			}
		}
		people = append(people, person)
	}
	for nextLink := getNextLink(resp); nextLink != ""; {
		nextResp, err := c.inner.Me().People().WithUrl(nextLink).Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("searching people: %w", err)
		}
		for _, p := range nextResp.GetValue() {
			person := Person{
				DisplayName: derefStr(p.GetDisplayName()),
				JobTitle:    derefStr(p.GetJobTitle()),
				Department:  derefStr(p.GetDepartment()),
				Company:     derefStr(p.GetCompanyName()),
			}
			if addrs := p.GetScoredEmailAddresses(); len(addrs) > 0 {
				if addrs[0].GetAddress() != nil {
					person.Email = *addrs[0].GetAddress()
				}
			}
			people = append(people, person)
		}
		nextLink = getNextLink(nextResp)
	}
	return people, nil
}
