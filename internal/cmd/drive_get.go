package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveGetCmd gets item details.
type DriveGetCmd struct {
	ID      string `arg:"" help:"Item ID"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveGetCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ID) {
		return fmt.Errorf("argument looks like a path; use 'olk drive ls %s' to find item IDs", c.ID)
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	item, err := client.GetDriveItem(ctx.Ctx, driveID, c.ID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(item, 1, "")
	}

	loc, _ := ctx.Timezone()
	fmt.Printf("ID:       %s\n", outfmt.Sanitize(item.ID))
	fmt.Printf("Name:     %s\n", outfmt.Sanitize(item.Name))
	fmt.Printf("Type:     %s\n", outfmt.Sanitize(item.ItemType))
	if item.ItemType == driveItemTypeFile {
		fmt.Printf("Size:     %s\n", formatBytes(item.Size))
		if item.MimeType != "" {
			fmt.Printf("MIME:     %s\n", outfmt.Sanitize(item.MimeType))
		}
	}
	if item.ItemType == driveItemTypeFolder && item.ChildCount > 0 {
		fmt.Printf("Children: %d\n", item.ChildCount)
	}
	fmt.Printf("Created:  %s\n", outfmt.Sanitize(outfmt.ConvertTime(item.CreatedAt, loc)))
	fmt.Printf("Modified: %s\n", outfmt.Sanitize(outfmt.ConvertTime(item.ModifiedAt, loc)))
	if item.CreatedBy != "" {
		fmt.Printf("Created by:  %s\n", outfmt.Sanitize(item.CreatedBy))
	}
	if item.ModifiedBy != "" {
		fmt.Printf("Modified by: %s\n", outfmt.Sanitize(item.ModifiedBy))
	}
	if item.ParentPath != "" {
		fmt.Printf("Parent:   %s\n", outfmt.Sanitize(item.ParentPath))
	}
	if item.WebURL != "" {
		fmt.Printf("URL:      %s\n", outfmt.Sanitize(item.WebURL))
	}
	return nil
}
