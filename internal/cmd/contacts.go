package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// formatAddress formats an address as a single line, omitting empty parts.
func formatAddress(addr *graphapi.Address) string {
	var parts []string
	if addr.Street != "" {
		parts = append(parts, addr.Street)
	}
	if addr.City != "" {
		parts = append(parts, addr.City)
	}
	if addr.State != "" {
		parts = append(parts, addr.State)
	}
	if addr.PostalCode != "" {
		parts = append(parts, addr.PostalCode)
	}
	if addr.CountryOrRegion != "" {
		parts = append(parts, addr.CountryOrRegion)
	}
	return strings.Join(parts, ", ")
}

// bestPhone returns the best available phone for table display (mobile > business > home).
func bestPhone(ct *graphapi.Contact) string {
	if ct.MobilePhone != "" {
		return ct.MobilePhone
	}
	if len(ct.BusinessPhones) > 0 {
		return ct.BusinessPhones[0]
	}
	if len(ct.HomePhones) > 0 {
		return ct.HomePhones[0]
	}
	return ""
}

type ContactsCmd struct {
	List   ContactsListCmd   `cmd:"" help:"List contacts"`
	Get    ContactsGetCmd    `cmd:"" help:"Get contact details"`
	Create ContactsCreateCmd `cmd:"" help:"Create a contact"`
	Update ContactsUpdateCmd `cmd:"" help:"Update a contact"`
	Delete ContactsDeleteCmd `cmd:"" help:"Delete a contact"`
	Search ContactsSearchCmd `cmd:"" help:"Search contacts"`
}

type ContactsListCmd struct {
	Top  int32  `help:"Max contacts to return" default:"25" short:"n"`
	Skip int32  `help:"Number of contacts to skip (offset pagination)" default:"0"`
	Sort string `help:"Sort by field" enum:"displayName,givenName,surname," default:""`
}

func (c *ContactsListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	contacts, err := client.ListContacts(ctx.Ctx, c.Top, c.Skip, c.Sort)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(contacts, len(contacts), "")
	}

	headers := []string{"ID", "NAME", "EMAIL", "PHONE", "COMPANY", "TITLE"}
	var rows [][]string
	for i := range contacts {
		ct := &contacts[i]
		id := outfmt.Truncate(ct.ID, 15)
		email := ""
		if len(ct.Emails) > 0 {
			email = ct.Emails[0]
		}
		rows = append(rows, []string{id, ct.DisplayName, email, bestPhone(ct), ct.Company, ct.JobTitle})
	}

	return printer.Print(headers, rows, contacts, len(contacts), "")
}

type ContactsGetCmd struct {
	ID string `arg:"" help:"Contact ID"`
}

func (c *ContactsGetCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	contact, err := client.GetContact(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(contact, 1, "")
	}

	fmt.Printf("Name:       %s\n", outfmt.Sanitize(contact.DisplayName))
	fmt.Printf("First:      %s\n", outfmt.Sanitize(contact.FirstName))
	if contact.MiddleName != "" {
		fmt.Printf("Middle:     %s\n", outfmt.Sanitize(contact.MiddleName))
	}
	fmt.Printf("Last:       %s\n", outfmt.Sanitize(contact.LastName))
	if contact.NickName != "" {
		fmt.Printf("Nickname:   %s\n", outfmt.Sanitize(contact.NickName))
	}
	if len(contact.Emails) > 0 {
		fmt.Printf("Email:      %s\n", outfmt.Sanitize(strings.Join(contact.Emails, ", ")))
	}
	if len(contact.BusinessPhones) > 0 {
		fmt.Printf("Business:   %s\n", outfmt.Sanitize(strings.Join(contact.BusinessPhones, ", ")))
	}
	if len(contact.HomePhones) > 0 {
		fmt.Printf("Home:       %s\n", outfmt.Sanitize(strings.Join(contact.HomePhones, ", ")))
	}
	if contact.MobilePhone != "" {
		fmt.Printf("Mobile:     %s\n", outfmt.Sanitize(contact.MobilePhone))
	}
	if len(contact.ImAddresses) > 0 {
		fmt.Printf("IM:         %s\n", outfmt.Sanitize(strings.Join(contact.ImAddresses, ", ")))
	}
	if contact.Company != "" {
		fmt.Printf("Company:    %s\n", outfmt.Sanitize(contact.Company))
	}
	if contact.JobTitle != "" {
		fmt.Printf("Title:      %s\n", outfmt.Sanitize(contact.JobTitle))
	}
	if contact.Department != "" {
		fmt.Printf("Department: %s\n", outfmt.Sanitize(contact.Department))
	}
	if contact.OfficeLocation != "" {
		fmt.Printf("Office:     %s\n", outfmt.Sanitize(contact.OfficeLocation))
	}
	if contact.Manager != "" {
		fmt.Printf("Manager:    %s\n", outfmt.Sanitize(contact.Manager))
	}
	if contact.AssistantName != "" {
		fmt.Printf("Assistant:  %s\n", outfmt.Sanitize(contact.AssistantName))
	}
	if contact.Profession != "" {
		fmt.Printf("Profession: %s\n", outfmt.Sanitize(contact.Profession))
	}
	if contact.Birthday != "" {
		fmt.Printf("Birthday:   %s\n", outfmt.Sanitize(contact.Birthday))
	}
	if contact.SpouseName != "" {
		fmt.Printf("Spouse:     %s\n", outfmt.Sanitize(contact.SpouseName))
	}
	if len(contact.Children) > 0 {
		fmt.Printf("Children:   %s\n", outfmt.Sanitize(strings.Join(contact.Children, ", ")))
	}
	if contact.BusinessHomePage != "" {
		fmt.Printf("Homepage:   %s\n", outfmt.Sanitize(contact.BusinessHomePage))
	}
	if contact.BusinessAddress != nil {
		fmt.Printf("Business Address: %s\n", outfmt.Sanitize(formatAddress(contact.BusinessAddress)))
	}
	if contact.HomeAddress != nil {
		fmt.Printf("Home Address:     %s\n", outfmt.Sanitize(formatAddress(contact.HomeAddress)))
	}
	if contact.OtherAddress != nil {
		fmt.Printf("Other Address:    %s\n", outfmt.Sanitize(formatAddress(contact.OtherAddress)))
	}
	if len(contact.Categories) > 0 {
		fmt.Printf("Categories: %s\n", outfmt.Sanitize(strings.Join(contact.Categories, ", ")))
	}
	if contact.PersonalNotes != "" {
		fmt.Printf("Notes:      %s\n", outfmt.Sanitize(contact.PersonalNotes))
	}

	return nil
}

type ContactsCreateCmd struct {
	FirstName     string   `help:"First name" required:""`
	LastName      string   `help:"Last name" required:""`
	MiddleName    string   `help:"Middle name" name:"middle-name"`
	NickName      string   `help:"Nickname" name:"nickname"`
	Email         []string `help:"Email address (repeatable)" short:"e"`
	MobilePhone   string   `help:"Mobile phone number" short:"p" name:"mobile-phone"`
	BusinessPhone string   `help:"Business phone number" name:"business-phone"`
	HomePhone     string   `help:"Home phone number" name:"home-phone"`
	Company       string   `help:"Company name" short:"c"`
	Title         string   `help:"Job title"`
	Department    string   `help:"Department" short:"d"`
	Manager       string   `help:"Manager name"`
	Birthday      string   `help:"Birthday (YYYY-MM-DD)"`
	Notes         string   `help:"Personal notes"`
	Categories    []string `help:"Category (repeatable)" short:"g"`
	Street        string   `help:"Street address"`
	City          string   `help:"City"`
	State         string   `help:"State or province"`
	PostalCode    string   `help:"Postal code" name:"postal-code"`
	Country       string   `help:"Country or region"`
	AddressType   string   `help:"Which address to set" enum:"business,home,other" default:"business" name:"address-type"`
}

func (c *ContactsCreateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		emailStr := strings.Join(c.Email, ", ")
		fmt.Printf("Would create contact: %s %s <%s>\n", outfmt.Sanitize(c.FirstName), outfmt.Sanitize(c.LastName), outfmt.Sanitize(emailStr))
		return nil
	}

	in := &graphapi.ContactCreateInput{
		FirstName:     c.FirstName,
		LastName:      c.LastName,
		MiddleName:    c.MiddleName,
		NickName:      c.NickName,
		Emails:        c.Email,
		BusinessPhone: c.BusinessPhone,
		HomePhone:     c.HomePhone,
		MobilePhone:   c.MobilePhone,
		Company:       c.Company,
		JobTitle:      c.Title,
		Department:    c.Department,
		Manager:       c.Manager,
		Birthday:      c.Birthday,
		PersonalNotes: c.Notes,
		Categories:    c.Categories,
		AddressType:   c.AddressType,
	}
	if c.Street != "" || c.City != "" || c.State != "" || c.PostalCode != "" || c.Country != "" {
		in.Address = &graphapi.Address{
			Street:          c.Street,
			City:            c.City,
			State:           c.State,
			PostalCode:      c.PostalCode,
			CountryOrRegion: c.Country,
		}
	}

	contact, err := client.CreateContact(ctx.Ctx, in)
	if err != nil {
		return err
	}

	fmt.Printf("Contact created: %s (ID: %s)\n", outfmt.Sanitize(contact.DisplayName), contact.ID)
	return nil
}

type ContactsUpdateCmd struct {
	ID            string   `arg:"" help:"Contact ID"`
	FirstName     string   `help:"First name"`
	LastName      string   `help:"Last name"`
	MiddleName    string   `help:"Middle name ('none' to clear)" name:"middle-name"`
	NickName      string   `help:"Nickname ('none' to clear)" name:"nickname"`
	Email         []string `help:"Email address (repeatable, replaces all; 'none' to clear)" short:"e"`
	MobilePhone   string   `help:"Mobile phone number ('none' to clear)" short:"p" name:"mobile-phone"`
	BusinessPhone string   `help:"Business phone number ('none' to clear)" name:"business-phone"`
	HomePhone     string   `help:"Home phone number ('none' to clear)" name:"home-phone"`
	Company       string   `help:"Company name ('none' to clear)" short:"c"`
	Title         string   `help:"Job title ('none' to clear)"`
	Department    string   `help:"Department ('none' to clear)" short:"d"`
	Manager       string   `help:"Manager name ('none' to clear)"`
	Birthday      string   `help:"Birthday (YYYY-MM-DD, 'none' to clear)"`
	Notes         string   `help:"Personal notes ('none' to clear)"`
	Categories    []string `help:"Category (repeatable, replaces all; 'none' to clear)" short:"g"`
	Street        string   `help:"Street address ('none' to clear)"`
	City          string   `help:"City ('none' to clear)"`
	State         string   `help:"State or province ('none' to clear)"`
	PostalCode    string   `help:"Postal code ('none' to clear)" name:"postal-code"`
	Country       string   `help:"Country or region ('none' to clear)"`
	AddressType   string   `help:"Which address to set" enum:"business,home,other" default:"business" name:"address-type"`
}

// rejectMixedClear returns an error if "none" is mixed with other values in a repeatable flag.
func rejectMixedClear(vals []string, flag string) error {
	if len(vals) > 1 {
		for _, v := range vals {
			if v == clearSentinel {
				return fmt.Errorf("%s: cannot mix 'none' with other values", flag)
			}
		}
	}
	return nil
}

// contactUpdateString maps a CLI flag value to an update pointer.
// Returns nil if the flag was not provided (empty), a pointer to empty string
// if the user passed "none" (clear), or a pointer to the value otherwise.
func contactUpdateString(v string) *string {
	if v == "" {
		return nil
	}
	if v == clearSentinel {
		empty := ""
		return &empty
	}
	return &v
}

func (c *ContactsUpdateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	in := &graphapi.ContactUpdateInput{
		AddressType:   c.AddressType,
		MiddleName:    contactUpdateString(c.MiddleName),
		NickName:      contactUpdateString(c.NickName),
		BusinessPhone: contactUpdateString(c.BusinessPhone),
		HomePhone:     contactUpdateString(c.HomePhone),
		MobilePhone:   contactUpdateString(c.MobilePhone),
		Company:       contactUpdateString(c.Company),
		JobTitle:      contactUpdateString(c.Title),
		Department:    contactUpdateString(c.Department),
		Manager:       contactUpdateString(c.Manager),
		Birthday:      contactUpdateString(c.Birthday),
		PersonalNotes: contactUpdateString(c.Notes),
	}
	// First/last name are identity fields — no 'none' sentinel support.
	if c.FirstName != "" {
		in.FirstName = &c.FirstName
	}
	if c.LastName != "" {
		in.LastName = &c.LastName
	}
	if len(c.Email) > 0 {
		if err := rejectMixedClear(c.Email, "--email"); err != nil {
			return err
		}
		if len(c.Email) == 1 && c.Email[0] == clearSentinel {
			empty := []string{}
			in.Emails = &empty
		} else {
			in.Emails = &c.Email
		}
	}
	if len(c.Categories) > 0 {
		if err := rejectMixedClear(c.Categories, "--categories"); err != nil {
			return err
		}
		if len(c.Categories) == 1 && c.Categories[0] == clearSentinel {
			empty := []string{}
			in.Categories = &empty
		} else {
			in.Categories = &c.Categories
		}
	}
	if c.Street != "" || c.City != "" || c.State != "" || c.PostalCode != "" || c.Country != "" {
		// Read-modify-write: fetch existing address, merge provided fields,
		// so unspecified fields are preserved. 'none' clears individual parts.
		existing, err := client.GetContact(ctx.Ctx, c.ID)
		if err != nil {
			return err
		}
		var base graphapi.Address
		switch c.AddressType {
		case "home":
			if existing.HomeAddress != nil {
				base = *existing.HomeAddress
			}
		case "other":
			if existing.OtherAddress != nil {
				base = *existing.OtherAddress
			}
		default:
			if existing.BusinessAddress != nil {
				base = *existing.BusinessAddress
			}
		}
		if c.Street != "" {
			if c.Street == clearSentinel {
				base.Street = ""
			} else {
				base.Street = c.Street
			}
		}
		if c.City != "" {
			if c.City == clearSentinel {
				base.City = ""
			} else {
				base.City = c.City
			}
		}
		if c.State != "" {
			if c.State == clearSentinel {
				base.State = ""
			} else {
				base.State = c.State
			}
		}
		if c.PostalCode != "" {
			if c.PostalCode == clearSentinel {
				base.PostalCode = ""
			} else {
				base.PostalCode = c.PostalCode
			}
		}
		if c.Country != "" {
			if c.Country == clearSentinel {
				base.CountryOrRegion = ""
			} else {
				base.CountryOrRegion = c.Country
			}
		}
		in.Address = &base
	}

	contact, err := client.UpdateContact(ctx.Ctx, c.ID, in)
	if err != nil {
		return err
	}

	fmt.Printf("Contact updated: %s\n", outfmt.Sanitize(contact.DisplayName))
	return nil
}

type ContactsDeleteCmd struct {
	ID string `arg:"" help:"Contact ID"`
}

func (c *ContactsDeleteCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete contact %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	err = client.DeleteContact(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Contact deleted.")
	return nil
}

type ContactsSearchCmd struct {
	Query string `arg:"" help:"Search query"`
	Top   int32  `help:"Max results" default:"25" short:"n"`
}

func (c *ContactsSearchCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	contacts, err := client.SearchContacts(ctx.Ctx, c.Query, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(contacts, len(contacts), "")
	}

	headers := []string{"ID", "NAME", "EMAIL", "PHONE", "COMPANY"}
	var rows [][]string
	for i := range contacts {
		ct := &contacts[i]
		id := outfmt.Truncate(ct.ID, 15)
		email := ""
		if len(ct.Emails) > 0 {
			email = ct.Emails[0]
		}
		rows = append(rows, []string{id, ct.DisplayName, email, bestPhone(ct), ct.Company})
	}

	return printer.Print(headers, rows, contacts, len(contacts), "")
}
