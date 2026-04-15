package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveMvCmd moves or renames a file or folder.
type DriveMvCmd struct {
	ItemID  string `arg:"" help:"Item ID to move or rename"`
	Dest    string `arg:"" help:"Destination folder path or new name"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveMvCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ItemID) {
		return fmt.Errorf("first argument looks like a path; use 'olk drive ls' to find item IDs")
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would move %s to %s\n", outfmt.Sanitize(c.ItemID), outfmt.Sanitize(c.Dest))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Determine if Dest is a path (move) or a plain name (rename)
	var destParentID, newName string
	if looksLikePath(c.Dest) {
		// Move to a new folder
		dest, err := client.ResolveItemByPath(ctx.Ctx, driveID, c.Dest)
		if err != nil {
			return fmt.Errorf("resolving destination path: %w", err)
		}
		destParentID = dest.ID
	} else {
		// Rename
		newName = c.Dest
	}

	item, err := client.MoveDriveItem(ctx.Ctx, driveID, c.ItemID, destParentID, newName)
	if err != nil {
		return err
	}

	fmt.Printf("Moved: %s (ID: %s)\n", outfmt.Sanitize(item.Name), outfmt.Sanitize(item.ID))
	return nil
}
