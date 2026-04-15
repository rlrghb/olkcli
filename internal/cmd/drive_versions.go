package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveVersionsCmd lists version history for a file.
type DriveVersionsCmd struct {
	ID      string `arg:"" help:"Item ID"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveVersionsCmd) Run(ctx *RunContext) error {
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

	versions, err := client.ListDriveItemVersions(ctx.Ctx, driveID, c.ID)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		fmt.Println("No versions.")
		return nil
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(versions, len(versions), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"VERSION", "MODIFIED", "SIZE", "MODIFIED BY"}
	var rows [][]string
	for _, v := range versions {
		rows = append(rows, []string{
			outfmt.Sanitize(v.ID),
			outfmt.Truncate(outfmt.Sanitize(outfmt.ConvertTime(v.ModifiedAt, loc)), 16),
			formatBytes(v.Size),
			outfmt.Sanitize(v.ModifiedBy),
		})
	}
	return printer.Print(headers, rows, versions, len(versions), "")
}
