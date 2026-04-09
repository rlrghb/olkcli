package graphapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// safeSearchQuery matches only alphanumeric, spaces, @, dots, hyphens, underscores.
var safeSearchQuery = regexp.MustCompile(`^[a-zA-Z0-9 @._-]+$`)

// Contact is a simplified contact representation
type Contact struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	FirstName   string   `json:"givenName"`
	LastName    string   `json:"surname"`
	Emails      []string `json:"emailAddresses"`
	Phones      []string `json:"phones,omitempty"`
	Company     string   `json:"companyName"`
	JobTitle    string   `json:"jobTitle"`
}

func (c *Client) ListContacts(ctx context.Context, top int32) ([]Contact, error) {
	top = clampTop(top)

	queryParams := &users.ItemContactsRequestBuilderGetQueryParameters{
		Top:    &top,
		Select: []string{"id", "displayName", "givenName", "surname", "emailAddresses", "businessPhones", "homePhones", "mobilePhone", "companyName", "jobTitle"},
	}

	config := &users.ItemContactsRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	resp, err := c.inner.Me().Contacts().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("listing contacts: %w", err)
	}

	var contacts []Contact
	for _, ct := range resp.GetValue() {
		contacts = append(contacts, convertContact(ct))
	}
	return contacts, nil
}

func (c *Client) GetContact(ctx context.Context, contactID string) (*Contact, error) {
	if err := validateID(contactID, "contact ID"); err != nil {
		return nil, err
	}
	ct, err := c.inner.Me().Contacts().ByContactId(contactID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting contact: %w", err)
	}
	contact := convertContact(ct)
	return &contact, nil
}

func (c *Client) CreateContact(ctx context.Context, firstName, lastName, email, phone, company, jobTitle string) (*Contact, error) {
	ct := models.NewContact()
	if firstName != "" {
		ct.SetGivenName(&firstName)
	}
	if lastName != "" {
		ct.SetSurname(&lastName)
	}
	if email != "" {
		if err := ValidateEmail(email); err != nil {
			return nil, fmt.Errorf("invalid contact email: %w", err)
		}
		addr := models.NewEmailAddress()
		addr.SetAddress(&email)
		ct.SetEmailAddresses([]models.EmailAddressable{addr})
	}
	if phone != "" {
		if err := ValidatePhone(phone); err != nil {
			return nil, fmt.Errorf("invalid contact phone: %w", err)
		}
		ct.SetBusinessPhones([]string{phone})
	}
	if company != "" {
		ct.SetCompanyName(&company)
	}
	if jobTitle != "" {
		ct.SetJobTitle(&jobTitle)
	}

	created, err := c.inner.Me().Contacts().Post(ctx, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("creating contact: %w", err)
	}
	contact := convertContact(created)
	return &contact, nil
}

func (c *Client) UpdateContact(ctx context.Context, contactID string, firstName, lastName, email, phone, company, jobTitle *string) (*Contact, error) {
	ct := models.NewContact()
	if firstName != nil {
		ct.SetGivenName(firstName)
	}
	if lastName != nil {
		ct.SetSurname(lastName)
	}
	if email != nil {
		if err := ValidateEmail(*email); err != nil {
			return nil, fmt.Errorf("invalid contact email: %w", err)
		}
		addr := models.NewEmailAddress()
		addr.SetAddress(email)
		ct.SetEmailAddresses([]models.EmailAddressable{addr})
	}
	if phone != nil {
		if err := ValidatePhone(*phone); err != nil {
			return nil, fmt.Errorf("invalid contact phone: %w", err)
		}
		ct.SetBusinessPhones([]string{*phone})
	}
	if company != nil {
		ct.SetCompanyName(company)
	}
	if jobTitle != nil {
		ct.SetJobTitle(jobTitle)
	}

	if err := validateID(contactID, "contact ID"); err != nil {
		return nil, err
	}
	updated, err := c.inner.Me().Contacts().ByContactId(contactID).Patch(ctx, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("updating contact: %w", err)
	}
	contact := convertContact(updated)
	return &contact, nil
}

func (c *Client) DeleteContact(ctx context.Context, contactID string) error {
	if err := validateID(contactID, "contact ID"); err != nil {
		return err
	}
	err := c.inner.Me().Contacts().ByContactId(contactID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting contact: %w", err)
	}
	return nil
}

func (c *Client) SearchContacts(ctx context.Context, query string, top int32) ([]Contact, error) {
	top = clampTop(top)
	// Strict allowlist: only safe characters permitted in search queries
	if !safeSearchQuery.MatchString(query) {
		return nil, fmt.Errorf("search query contains invalid characters: only letters, numbers, spaces, @, dots, hyphens, and underscores are allowed")
	}
	// Defense-in-depth: escape single quotes even though regex blocks them
	escaped := strings.ReplaceAll(query, "'", "''")
	filter := fmt.Sprintf("startswith(displayName,'%s') or startswith(givenName,'%s') or startswith(surname,'%s')", escaped, escaped, escaped)

	queryParams := &users.ItemContactsRequestBuilderGetQueryParameters{
		Top:    &top,
		Filter: &filter,
		Select: []string{"id", "displayName", "givenName", "surname", "emailAddresses", "businessPhones", "homePhones", "mobilePhone", "companyName", "jobTitle"},
	}

	config := &users.ItemContactsRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	resp, err := c.inner.Me().Contacts().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("searching contacts: %w", err)
	}

	var contacts []Contact
	for _, ct := range resp.GetValue() {
		contacts = append(contacts, convertContact(ct))
	}
	return contacts, nil
}

func convertContact(ct models.Contactable) Contact {
	contact := Contact{}
	if ct.GetId() != nil {
		contact.ID = *ct.GetId()
	}
	if ct.GetDisplayName() != nil {
		contact.DisplayName = *ct.GetDisplayName()
	}
	if ct.GetGivenName() != nil {
		contact.FirstName = *ct.GetGivenName()
	}
	if ct.GetSurname() != nil {
		contact.LastName = *ct.GetSurname()
	}
	for _, e := range ct.GetEmailAddresses() {
		if e.GetAddress() != nil {
			contact.Emails = append(contact.Emails, *e.GetAddress())
		}
	}
	for _, p := range ct.GetBusinessPhones() {
		contact.Phones = append(contact.Phones, p)
	}
	for _, p := range ct.GetHomePhones() {
		contact.Phones = append(contact.Phones, p)
	}
	if ct.GetMobilePhone() != nil {
		contact.Phones = append(contact.Phones, *ct.GetMobilePhone())
	}
	if ct.GetCompanyName() != nil {
		contact.Company = *ct.GetCompanyName()
	}
	if ct.GetJobTitle() != nil {
		contact.JobTitle = *ct.GetJobTitle()
	}
	return contact
}
