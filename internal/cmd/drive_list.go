package cmd

import (
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// DriveListCmd lists all drives for the current user.
type DriveListCmd struct{}

func (c *DriveListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	drives, err := client.ListDrives(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(drives, len(drives), "")
	}

	headers := []string{"ID", "NAME", "TYPE", "USED", "TOTAL"}
	var rows [][]string
	for i := range drives {
		d := &drives[i]
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(d.ID), 15),
			outfmt.Sanitize(d.Name),
			outfmt.Sanitize(d.DriveType),
			formatBytes(d.QuotaUsed),
			formatBytes(d.QuotaTotal),
		})
	}
	return printer.Print(headers, rows, drives, len(drives), "")
}
