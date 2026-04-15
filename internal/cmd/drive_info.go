package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveInfoCmd shows drive details and quota.
type DriveInfoCmd struct {
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveInfoCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	drive, err := client.GetDrive(ctx.Ctx, c.DriveID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(drive, 1, "")
	}

	fmt.Printf("ID:        %s\n", outfmt.Sanitize(drive.ID))
	fmt.Printf("Name:      %s\n", outfmt.Sanitize(drive.Name))
	fmt.Printf("Type:      %s\n", outfmt.Sanitize(drive.DriveType))
	if drive.OwnerName != "" {
		fmt.Printf("Owner:     %s\n", outfmt.Sanitize(drive.OwnerName))
	}
	if drive.OwnerEmail != "" {
		fmt.Printf("Email:     %s\n", outfmt.Sanitize(drive.OwnerEmail))
	}
	fmt.Printf("Used:      %s\n", formatBytes(drive.QuotaUsed))
	fmt.Printf("Total:     %s\n", formatBytes(drive.QuotaTotal))
	fmt.Printf("Remaining: %s\n", formatBytes(drive.QuotaRemaining))
	fmt.Printf("State:     %s\n", outfmt.Sanitize(drive.QuotaState))
	if drive.WebURL != "" {
		fmt.Printf("URL:       %s\n", outfmt.Sanitize(drive.WebURL))
	}
	return nil
}
