package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailMoveCmd struct {
	ID     string `arg:"" help:"Message ID"`
	Folder string `arg:"" help:"Destination folder ID or well-known name"`
}

func (c *MailMoveCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would move message %s to folder %s\n", outfmt.Sanitize(c.ID), outfmt.Sanitize(c.Folder))
		return nil
	}

	err = client.MoveMessage(ctx.Ctx, c.ID, c.Folder)
	if err != nil {
		return err
	}

	fmt.Printf("Message moved to %s.\n", outfmt.Sanitize(c.Folder))
	return nil
}
