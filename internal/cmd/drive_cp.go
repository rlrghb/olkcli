package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveCpCmd copies a file or folder.
type DriveCpCmd struct {
	ItemID  string `arg:"" help:"Source item ID"`
	Dest    string `arg:"" help:"Destination folder path"`
	Name    string `help:"New name for the copy" short:"n"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveCpCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ItemID) {
		return fmt.Errorf("first argument looks like a path; use 'olk drive ls %s' to find item IDs", c.ItemID)
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		name := c.Name
		if name == "" {
			name = "(same name)"
		}
		fmt.Printf("Would copy %s to %s as %s\n",
			outfmt.Sanitize(c.ItemID), outfmt.Sanitize(c.Dest), outfmt.Sanitize(name))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Resolve destination path to item ID
	dest, err := client.ResolveItemByPath(ctx.Ctx, driveID, c.Dest)
	if err != nil {
		return fmt.Errorf("resolving destination path: %w", err)
	}

	err = client.CopyDriveItem(ctx.Ctx, driveID, c.ItemID, dest.ID, c.Name)
	if err != nil {
		return err
	}

	fmt.Println("Copy initiated.")
	return nil
}
