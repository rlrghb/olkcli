package cmd

// DriveLsCmd lists folder contents.
type DriveLsCmd struct {
	Path    string `arg:"" optional:"" help:"Folder path (default: root)" default:"/"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
	Top     int32  `help:"Number of items to return" default:"50" short:"n"`
}

func (c *DriveLsCmd) Run(ctx *RunContext) error {
	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	items, err := client.ListDriveChildrenByPath(ctx.Ctx, driveID, c.Path, c.Top)
	if err != nil {
		return err
	}

	return printDriveItems(ctx, items)
}
