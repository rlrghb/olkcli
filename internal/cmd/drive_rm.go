package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveRmCmd deletes a file or folder.
type DriveRmCmd struct {
	ID      string `arg:"" help:"Item ID to delete"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveRmCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ID) {
		return fmt.Errorf("argument looks like a path; use 'olk drive ls %s' to find item IDs", c.ID)
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete item %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ID, 30)))
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete item %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.DeleteDriveItem(ctx.Ctx, driveID, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Item deleted.")
	return nil
}
