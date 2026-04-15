package cmd

// DriveSharedCmd lists files shared with the current user.
type DriveSharedCmd struct {
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveSharedCmd) Run(ctx *RunContext) error {
	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	items, err := client.SharedWithMeItems(ctx.Ctx, driveID)
	if err != nil {
		return err
	}

	return printDriveItems(ctx, items)
}
