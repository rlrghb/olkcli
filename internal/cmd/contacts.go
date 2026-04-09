package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// bestPhone returns the best available phone for table display (mobile > business > home).
func bestPhone(ct graphapi.Contact) string {
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
	Top int32 `help:"Max contacts to return" default:"25" short:"n"`
}

func (c *ContactsListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	contacts, err := client.ListContacts(ctx.Ctx, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(contacts, len(contacts), "")
	}

	headers := []string{"ID", "NAME", "EMAIL", "PHONE", "COMPANY", "TITLE"}
	var rows [][]string
	for _, ct := range contacts {
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

	fmt.Printf("Name:    %s\n", outfmt.Sanitize(contact.DisplayName))
	fmt.Printf("First:   %s\n", outfmt.Sanitize(contact.FirstName))
	fmt.Printf("Last:    %s\n", outfmt.Sanitize(contact.LastName))
	if len(contact.Emails) > 0 {
		fmt.Printf("Email:   %s\n", outfmt.Sanitize(strings.Join(contact.Emails, ", ")))
	}
	if len(contact.BusinessPhones) > 0 {
		fmt.Printf("Business: %s\n", outfmt.Sanitize(strings.Join(contact.BusinessPhones, ", ")))
	}
	if len(contact.HomePhones) > 0 {
		fmt.Printf("Home:     %s\n", outfmt.Sanitize(strings.Join(contact.HomePhones, ", ")))
	}
	if contact.MobilePhone != "" {
		fmt.Printf("Mobile:   %s\n", outfmt.Sanitize(contact.MobilePhone))
	}
	if contact.Company != "" {
		fmt.Printf("Company: %s\n", outfmt.Sanitize(contact.Company))
	}
	if contact.JobTitle != "" {
		fmt.Printf("Title:   %s\n", outfmt.Sanitize(contact.JobTitle))
	}

	return nil
}

type ContactsCreateCmd struct {
	FirstName     string `help:"First name" required:""`
	LastName      string `help:"Last name" required:""`
	Email         string `help:"Email address" short:"e"`
	MobilePhone   string `help:"Mobile phone number" short:"p" name:"mobile-phone"`
	BusinessPhone string `help:"Business phone number" name:"business-phone"`
	HomePhone     string `help:"Home phone number" name:"home-phone"`
	Company       string `help:"Company name" short:"c"`
	Title         string `help:"Job title"`
}

func (c *ContactsCreateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create contact: %s %s <%s>\n", outfmt.Sanitize(c.FirstName), outfmt.Sanitize(c.LastName), outfmt.Sanitize(c.Email))
		return nil
	}

	contact, err := client.CreateContact(ctx.Ctx, c.FirstName, c.LastName, c.Email, c.BusinessPhone, c.HomePhone, c.MobilePhone, c.Company, c.Title)
	if err != nil {
		return err
	}

	fmt.Printf("Contact created: %s (ID: %s)\n", outfmt.Sanitize(contact.DisplayName), contact.ID)
	return nil
}

type ContactsUpdateCmd struct {
	ID            string `arg:"" help:"Contact ID"`
	FirstName     string `help:"First name"`
	LastName      string `help:"Last name"`
	Email         string `help:"Email address" short:"e"`
	MobilePhone   string `help:"Mobile phone number" short:"p" name:"mobile-phone"`
	BusinessPhone string `help:"Business phone number" name:"business-phone"`
	HomePhone     string `help:"Home phone number" name:"home-phone"`
	Company       string `help:"Company name" short:"c"`
	Title         string `help:"Job title"`
}

func (c *ContactsUpdateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	var firstName, lastName, email, businessPhone, homePhone, mobilePhone, company, title *string
	if c.FirstName != "" {
		firstName = &c.FirstName
	}
	if c.LastName != "" {
		lastName = &c.LastName
	}
	if c.Email != "" {
		email = &c.Email
	}
	if c.BusinessPhone != "" {
		businessPhone = &c.BusinessPhone
	}
	if c.HomePhone != "" {
		homePhone = &c.HomePhone
	}
	if c.MobilePhone != "" {
		mobilePhone = &c.MobilePhone
	}
	if c.Company != "" {
		company = &c.Company
	}
	if c.Title != "" {
		title = &c.Title
	}

	contact, err := client.UpdateContact(ctx.Ctx, c.ID, firstName, lastName, email, businessPhone, homePhone, mobilePhone, company, title)
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
	for _, ct := range contacts {
		id := outfmt.Truncate(ct.ID, 15)
		email := ""
		if len(ct.Emails) > 0 {
			email = ct.Emails[0]
		}
		rows = append(rows, []string{id, ct.DisplayName, email, bestPhone(ct), ct.Company})
	}

	return printer.Print(headers, rows, contacts, len(contacts), "")
}
