package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// WhoamiCmd displays the current user's profile information
type WhoamiCmd struct{}

func (c *WhoamiCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	profile, err := client.GetProfile(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(profile, 1, "")
	}

	fmt.Printf("Display Name: %s\n", outfmt.Sanitize(profile.DisplayName))
	fmt.Printf("Email:        %s\n", outfmt.Sanitize(profile.Email))
	fmt.Printf("UPN:          %s\n", outfmt.Sanitize(profile.UPN))
	if profile.JobTitle != "" {
		fmt.Printf("Job Title:    %s\n", outfmt.Sanitize(profile.JobTitle))
	}
	if profile.Department != "" {
		fmt.Printf("Department:   %s\n", outfmt.Sanitize(profile.Department))
	}
	if profile.Office != "" {
		fmt.Printf("Office:       %s\n", outfmt.Sanitize(profile.Office))
	}
	if profile.Phone != "" {
		fmt.Printf("Phone:        %s\n", outfmt.Sanitize(profile.Phone))
	}

	return nil
}
