package cmd

// DriveSearchCmd searches files by name or content.
type DriveSearchCmd struct {
	Query   string `arg:"" help:"Search query"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
	Top     int32  `help:"Number of results" default:"25" short:"n"`
}

func (c *DriveSearchCmd) Run(ctx *RunContext) error {
	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	items, err := client.SearchDrive(ctx.Ctx, driveID, c.Query, c.Top)
	if err != nil {
		return err
	}

	return printDriveItems(ctx, items)
}
