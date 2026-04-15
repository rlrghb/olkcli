package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveShareCmd creates a sharing link for an item.
type DriveShareCmd struct {
	ID      string `arg:"" help:"Item ID"`
	Type    string `help:"Link type" enum:"view,edit" default:"view" short:"t"`
	Scope   string `help:"Link scope" enum:"anonymous,organization" default:"anonymous" short:"s"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveShareCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ID) {
		return fmt.Errorf("argument looks like a path; use 'olk drive ls %s' to find item IDs", c.ID)
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create %s sharing link (%s) for %s\n",
			outfmt.Sanitize(c.Type), outfmt.Sanitize(c.Scope), outfmt.Sanitize(c.ID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	link, err := client.CreateShareLink(ctx.Ctx, driveID, c.ID, c.Type, c.Scope)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(link, 1, "")
	}

	fmt.Println(outfmt.Sanitize(link.URL))
	return nil
}
