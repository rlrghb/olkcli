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

// allowedContactOrderBy is the set of valid $orderby values for contacts.
var allowedContactOrderBy = map[string]bool{
	"displayName": true,
	"givenName":   true,
	"surname":     true,
}

// contactSelectFields is the list of fields to fetch from the Graph API for contacts.
var contactSelectFields = []string{
	"id", "displayName", "givenName", "surname", "middleName", "nickName",
	"emailAddresses", "businessPhones", "homePhones", "mobilePhone", "imAddresses",
	"companyName", "jobTitle", "department", "officeLocation", "profession", "manager", "assistantName",
	"birthday", "personalNotes", "spouseName", "children", "categories",
	"businessHomePage", "businessAddress", "homeAddress", "otherAddress",
}

// buildEmailAddresses validates and converts a slice of email strings to Graph EmailAddressable objects.
func buildEmailAddresses(emails []string) ([]models.EmailAddressable, error) {
	var addrs []models.EmailAddressable
	for _, e := range emails {
		if e == "" {
			continue
		}
		if err := ValidateEmail(e); err != nil {
			return nil, fmt.Errorf("invalid contact email: %w", err)
		}
		addr := models.NewEmailAddress()
		addr.SetAddress(&e)
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// Address is a physical address.
type Address struct {
	Street          string `json:"street,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
	CountryOrRegion string `json:"countryOrRegion,omitempty"`
}

// Contact is a simplified contact representation
type Contact struct {
	ID               string   `json:"id"`
	DisplayName      string   `json:"displayName"`
	FirstName        string   `json:"givenName"`
	LastName         string   `json:"surname"`
	MiddleName       string   `json:"middleName,omitempty"`
	NickName         string   `json:"nickName,omitempty"`
	Emails           []string `json:"emailAddresses"`
	BusinessPhones   []string `json:"businessPhones,omitempty"`
	HomePhones       []string `json:"homePhones,omitempty"`
	MobilePhone      string   `json:"mobilePhone,omitempty"`
	ImAddresses      []string `json:"imAddresses,omitempty"`
	Company          string   `json:"companyName"`
	JobTitle         string   `json:"jobTitle"`
	Department       string   `json:"department,omitempty"`
	OfficeLocation   string   `json:"officeLocation,omitempty"`
	Profession       string   `json:"profession,omitempty"`
	Manager          string   `json:"manager,omitempty"`
	AssistantName    string   `json:"assistantName,omitempty"`
	Birthday         string   `json:"birthday,omitempty"`
	PersonalNotes    string   `json:"personalNotes,omitempty"`
	SpouseName       string   `json:"spouseName,omitempty"`
	Children         []string `json:"children,omitempty"`
	Categories       []string `json:"categories,omitempty"`
	BusinessHomePage string   `json:"businessHomePage,omitempty"`
	BusinessAddress  *Address `json:"businessAddress,omitempty"`
	HomeAddress      *Address `json:"homeAddress,omitempty"`
	OtherAddress     *Address `json:"otherAddress,omitempty"`
}

func (c *Client) ListContacts(ctx context.Context, top, skip int32, orderBy string) ([]Contact, error) {
	top = clampTop(top)

	queryParams := &users.ItemContactsRequestBuilderGetQueryParameters{
		Top:    &top,
		Select: contactSelectFields,
	}
	if skip > 0 {
		queryParams.Skip = &skip
	}
	if orderBy != "" {
		if !allowedContactOrderBy[orderBy] {
			return nil, fmt.Errorf("invalid orderBy value: %q", orderBy)
		}
		queryParams.Orderby = []string{orderBy}
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

// ContactCreateInput contains the fields for creating a contact.
type ContactCreateInput struct {
	FirstName     string
	LastName      string
	Emails        []string
	BusinessPhone string
	HomePhone     string
	MobilePhone   string
	Company       string
	JobTitle      string
	Department    string
	Birthday      string
	PersonalNotes string
	Manager       string
	MiddleName    string
	NickName      string
	Categories    []string
	Address       *Address
	AddressType   string // "business", "home", "other"
}

// ContactUpdateInput contains the fields for updating a contact. Nil pointers mean "don't change".
type ContactUpdateInput struct {
	FirstName     *string
	LastName      *string
	Emails        *[]string
	BusinessPhone *string
	HomePhone     *string
	MobilePhone   *string
	Company       *string
	JobTitle      *string
	Department    *string
	Birthday      *string
	PersonalNotes *string
	Manager       *string
	MiddleName    *string
	NickName      *string
	Categories    *[]string
	Address       *Address
	AddressType   string
}

// validateContactFieldLengths checks that string fields don't exceed safe limits.
func validateContactFieldLengths(fields map[string]string) error {
	for label, value := range fields {
		if value == "" {
			continue
		}
		if err := ValidateContactFieldLen(value, label, maxContactFieldLen); err != nil {
			return err
		}
	}
	return nil
}

// validateCategories checks that each category is within length limits.
func validateCategories(cats []string) error {
	for _, c := range cats {
		if err := ValidateContactFieldLen(c, "category", maxContactFieldLen); err != nil {
			return err
		}
	}
	return nil
}

// applyContactFields sets fields on a Graph Contact model for contact creation.
// Empty strings are skipped (not set). For updates, use applyContactUpdateFields.
func applyContactFields(ct models.Contactable, in *ContactCreateInput) error {
	if err := validateContactFieldLengths(map[string]string{
		"first name":  in.FirstName,
		"last name":   in.LastName,
		"middle name": in.MiddleName,
		"nickname":    in.NickName,
		"company":     in.Company,
		"job title":   in.JobTitle,
		"department":  in.Department,
		"manager":     in.Manager,
	}); err != nil {
		return err
	}
	if in.PersonalNotes != "" {
		if err := ValidateContactFieldLen(in.PersonalNotes, "personal notes", maxContactNotesLen); err != nil {
			return err
		}
	}
	if in.Address != nil {
		if err := validateContactFieldLengths(map[string]string{
			"street":      in.Address.Street,
			"city":        in.Address.City,
			"state":       in.Address.State,
			"postal code": in.Address.PostalCode,
			"country":     in.Address.CountryOrRegion,
		}); err != nil {
			return err
		}
	}
	if in.FirstName != "" {
		ct.SetGivenName(&in.FirstName)
	}
	if in.LastName != "" {
		ct.SetSurname(&in.LastName)
	}
	if len(in.Emails) > 0 {
		addrs, err := buildEmailAddresses(in.Emails)
		if err != nil {
			return err
		}
		ct.SetEmailAddresses(addrs)
	}
	if in.BusinessPhone != "" {
		if err := ValidatePhone(in.BusinessPhone); err != nil {
			return fmt.Errorf("invalid business phone: %w", err)
		}
		ct.SetBusinessPhones([]string{in.BusinessPhone})
	}
	if in.HomePhone != "" {
		if err := ValidatePhone(in.HomePhone); err != nil {
			return fmt.Errorf("invalid home phone: %w", err)
		}
		ct.SetHomePhones([]string{in.HomePhone})
	}
	if in.MobilePhone != "" {
		if err := ValidatePhone(in.MobilePhone); err != nil {
			return fmt.Errorf("invalid mobile phone: %w", err)
		}
		ct.SetMobilePhone(&in.MobilePhone)
	}
	if in.Company != "" {
		ct.SetCompanyName(&in.Company)
	}
	if in.JobTitle != "" {
		ct.SetJobTitle(&in.JobTitle)
	}
	if in.Department != "" {
		ct.SetDepartment(&in.Department)
	}
	if in.Birthday != "" {
		t, err := ValidateBirthday(in.Birthday)
		if err != nil {
			return err
		}
		ct.SetBirthday(&t)
	}
	if in.PersonalNotes != "" {
		ct.SetPersonalNotes(&in.PersonalNotes)
	}
	if in.Manager != "" {
		ct.SetManager(&in.Manager)
	}
	if in.MiddleName != "" {
		ct.SetMiddleName(&in.MiddleName)
	}
	if in.NickName != "" {
		ct.SetNickName(&in.NickName)
	}
	if len(in.Categories) > 0 {
		if err := validateCategories(in.Categories); err != nil {
			return err
		}
		ct.SetCategories(in.Categories)
	}
	if in.Address != nil {
		addr := buildPhysicalAddress(in.Address)
		switch in.AddressType {
		case "home":
			ct.SetHomeAddress(addr)
		case "other":
			ct.SetOtherAddress(addr)
		default:
			ct.SetBusinessAddress(addr)
		}
	}
	return nil
}

// buildPhysicalAddress creates a Graph address, skipping empty fields (for create).
func buildPhysicalAddress(a *Address) models.PhysicalAddressable {
	addr := models.NewPhysicalAddress()
	if a.Street != "" {
		addr.SetStreet(&a.Street)
	}
	if a.City != "" {
		addr.SetCity(&a.City)
	}
	if a.State != "" {
		addr.SetState(&a.State)
	}
	if a.PostalCode != "" {
		addr.SetPostalCode(&a.PostalCode)
	}
	if a.CountryOrRegion != "" {
		addr.SetCountryOrRegion(&a.CountryOrRegion)
	}
	return addr
}

// buildPhysicalAddressComplete creates a Graph address setting all fields
// including empty strings (for update, so cleared fields are sent to Graph).
func buildPhysicalAddressComplete(a *Address) models.PhysicalAddressable {
	addr := models.NewPhysicalAddress()
	addr.SetStreet(&a.Street)
	addr.SetCity(&a.City)
	addr.SetState(&a.State)
	addr.SetPostalCode(&a.PostalCode)
	addr.SetCountryOrRegion(&a.CountryOrRegion)
	return addr
}

func (c *Client) CreateContact(ctx context.Context, in *ContactCreateInput) (*Contact, error) {
	ct := models.NewContact()
	if err := applyContactFields(ct, in); err != nil {
		return nil, err
	}

	created, err := c.inner.Me().Contacts().Post(ctx, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("creating contact: %w", err)
	}
	contact := convertContact(created)
	return &contact, nil
}

// applyContactUpdateFields sets fields on a Graph Contact model from update input.
// Unlike applyContactFields, this checks pointer presence (non-nil) rather than
// value emptiness, so that empty values can clear fields.
func applyContactUpdateFields(ct models.Contactable, in *ContactUpdateInput) error {
	// Validate lengths for non-nil, non-empty fields.
	lenChecks := map[string]string{}
	for label, ptr := range map[string]*string{
		"first name":  in.FirstName,
		"last name":   in.LastName,
		"middle name": in.MiddleName,
		"nickname":    in.NickName,
		"company":     in.Company,
		"job title":   in.JobTitle,
		"department":  in.Department,
		"manager":     in.Manager,
	} {
		if ptr != nil {
			lenChecks[label] = *ptr
		}
	}
	if err := validateContactFieldLengths(lenChecks); err != nil {
		return err
	}
	if in.PersonalNotes != nil && *in.PersonalNotes != "" {
		if err := ValidateContactFieldLen(*in.PersonalNotes, "personal notes", maxContactNotesLen); err != nil {
			return err
		}
	}
	if in.Address != nil {
		if err := validateContactFieldLengths(map[string]string{
			"street":      in.Address.Street,
			"city":        in.Address.City,
			"state":       in.Address.State,
			"postal code": in.Address.PostalCode,
			"country":     in.Address.CountryOrRegion,
		}); err != nil {
			return err
		}
	}
	if in.FirstName != nil {
		ct.SetGivenName(in.FirstName)
	}
	if in.LastName != nil {
		ct.SetSurname(in.LastName)
	}
	if in.Emails != nil {
		if len(*in.Emails) == 0 {
			ct.SetEmailAddresses([]models.EmailAddressable{})
		} else {
			addrs, err := buildEmailAddresses(*in.Emails)
			if err != nil {
				return err
			}
			ct.SetEmailAddresses(addrs)
		}
	}
	if in.BusinessPhone != nil {
		if *in.BusinessPhone != "" {
			if err := ValidatePhone(*in.BusinessPhone); err != nil {
				return fmt.Errorf("invalid business phone: %w", err)
			}
			ct.SetBusinessPhones([]string{*in.BusinessPhone})
		} else {
			ct.SetBusinessPhones([]string{})
		}
	}
	if in.HomePhone != nil {
		if *in.HomePhone != "" {
			if err := ValidatePhone(*in.HomePhone); err != nil {
				return fmt.Errorf("invalid home phone: %w", err)
			}
			ct.SetHomePhones([]string{*in.HomePhone})
		} else {
			ct.SetHomePhones([]string{})
		}
	}
	if in.MobilePhone != nil {
		if *in.MobilePhone != "" {
			if err := ValidatePhone(*in.MobilePhone); err != nil {
				return fmt.Errorf("invalid mobile phone: %w", err)
			}
		}
		ct.SetMobilePhone(in.MobilePhone)
	}
	if in.Company != nil {
		ct.SetCompanyName(in.Company)
	}
	if in.JobTitle != nil {
		ct.SetJobTitle(in.JobTitle)
	}
	if in.Department != nil {
		ct.SetDepartment(in.Department)
	}
	if in.Birthday != nil {
		if *in.Birthday != "" {
			t, err := ValidateBirthday(*in.Birthday)
			if err != nil {
				return err
			}
			ct.SetBirthday(&t)
		} else {
			ct.SetBirthday(nil)
		}
	}
	if in.PersonalNotes != nil {
		ct.SetPersonalNotes(in.PersonalNotes)
	}
	if in.Manager != nil {
		ct.SetManager(in.Manager)
	}
	if in.MiddleName != nil {
		ct.SetMiddleName(in.MiddleName)
	}
	if in.NickName != nil {
		ct.SetNickName(in.NickName)
	}
	if in.Categories != nil {
		if len(*in.Categories) > 0 {
			if err := validateCategories(*in.Categories); err != nil {
				return err
			}
		}
		ct.SetCategories(*in.Categories)
	}
	if in.Address != nil {
		addr := buildPhysicalAddressComplete(in.Address)
		switch in.AddressType {
		case "home":
			ct.SetHomeAddress(addr)
		case "other":
			ct.SetOtherAddress(addr)
		default:
			ct.SetBusinessAddress(addr)
		}
	}
	return nil
}

func (c *Client) UpdateContact(ctx context.Context, contactID string, in *ContactUpdateInput) (*Contact, error) {
	if err := validateID(contactID, "contact ID"); err != nil {
		return nil, err
	}

	ct := models.NewContact()
	if err := applyContactUpdateFields(ct, in); err != nil {
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
		Select: contactSelectFields,
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

func convertAddress(addr models.PhysicalAddressable) *Address {
	if addr == nil {
		return nil
	}
	a := Address{}
	if addr.GetStreet() != nil {
		a.Street = *addr.GetStreet()
	}
	if addr.GetCity() != nil {
		a.City = *addr.GetCity()
	}
	if addr.GetState() != nil {
		a.State = *addr.GetState()
	}
	if addr.GetPostalCode() != nil {
		a.PostalCode = *addr.GetPostalCode()
	}
	if addr.GetCountryOrRegion() != nil {
		a.CountryOrRegion = *addr.GetCountryOrRegion()
	}
	if a == (Address{}) {
		return nil
	}
	return &a
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
	if ct.GetMiddleName() != nil {
		contact.MiddleName = *ct.GetMiddleName()
	}
	if ct.GetNickName() != nil {
		contact.NickName = *ct.GetNickName()
	}
	for _, e := range ct.GetEmailAddresses() {
		if e.GetAddress() != nil {
			contact.Emails = append(contact.Emails, *e.GetAddress())
		}
	}
	contact.BusinessPhones = ct.GetBusinessPhones()
	contact.HomePhones = ct.GetHomePhones()
	if ct.GetMobilePhone() != nil {
		contact.MobilePhone = *ct.GetMobilePhone()
	}
	contact.ImAddresses = ct.GetImAddresses()
	if ct.GetCompanyName() != nil {
		contact.Company = *ct.GetCompanyName()
	}
	if ct.GetJobTitle() != nil {
		contact.JobTitle = *ct.GetJobTitle()
	}
	if ct.GetDepartment() != nil {
		contact.Department = *ct.GetDepartment()
	}
	if ct.GetOfficeLocation() != nil {
		contact.OfficeLocation = *ct.GetOfficeLocation()
	}
	if ct.GetProfession() != nil {
		contact.Profession = *ct.GetProfession()
	}
	if ct.GetManager() != nil {
		contact.Manager = *ct.GetManager()
	}
	if ct.GetAssistantName() != nil {
		contact.AssistantName = *ct.GetAssistantName()
	}
	if ct.GetBirthday() != nil {
		contact.Birthday = ct.GetBirthday().Format("2006-01-02")
	}
	if ct.GetPersonalNotes() != nil {
		contact.PersonalNotes = *ct.GetPersonalNotes()
	}
	if ct.GetSpouseName() != nil {
		contact.SpouseName = *ct.GetSpouseName()
	}
	contact.Children = ct.GetChildren()
	contact.Categories = ct.GetCategories()
	if ct.GetBusinessHomePage() != nil {
		contact.BusinessHomePage = *ct.GetBusinessHomePage()
	}
	contact.BusinessAddress = convertAddress(ct.GetBusinessAddress())
	contact.HomeAddress = convertAddress(ct.GetHomeAddress())
	contact.OtherAddress = convertAddress(ct.GetOtherAddress())
	return contact
}
