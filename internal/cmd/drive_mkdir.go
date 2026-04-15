package cmd

import (
	"fmt"
	"path"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveMkdirCmd creates a folder.
type DriveMkdirCmd struct {
	Path    string `arg:"" help:"Folder path to create (e.g. /Documents/NewFolder)"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveMkdirCmd) Run(ctx *RunContext) error {
	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	// Split path into parent and folder name
	parentPath := path.Dir(c.Path)
	folderName := path.Base(c.Path)
	if folderName == "" || folderName == "/" || folderName == "." {
		return fmt.Errorf("invalid folder path: %s", c.Path)
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create folder %q in %s\n", outfmt.Sanitize(folderName), outfmt.Sanitize(parentPath))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Resolve parent to item ID
	parentID := "root"
	if parentPath != "/" && parentPath != "." {
		parent, err := client.ResolveItemByPath(ctx.Ctx, driveID, parentPath)
		if err != nil {
			return fmt.Errorf("resolving parent path: %w", err)
		}
		parentID = parent.ID
	}

	folder, err := client.CreateFolder(ctx.Ctx, driveID, parentID, folderName)
	if err != nil {
		return err
	}

	fmt.Printf("Folder created: %s (ID: %s)\n", outfmt.Sanitize(folder.Name), outfmt.Sanitize(folder.ID))
	return nil
}
