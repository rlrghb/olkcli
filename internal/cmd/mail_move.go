package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailMoveCmd struct {
	ID     string `arg:"" help:"Message ID"`
	Folder string `arg:"" help:"Destination folder ID, well-known name, or path (for example Inbox/2026)"`
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

	// MoveMessage is intentionally an own-mailbox operation, so resolve a path
	// against that same mailbox rather than the global delegated read target.
	folderID, err := client.ResolveMailFolderPath(ctx.Ctx, "", c.Folder)
	if err != nil {
		return err
	}
	receipt, err := client.MoveMessage(ctx.Ctx, c.ID, folderID)
	if err != nil {
		return err
	}

	if ctx.Flags.JSON {
		return ctx.Printer().PrintJSON([]*graphapi.MoveMessageReceipt{receipt}, 1, "")
	}
	fmt.Printf("Message moved to %s.\n", outfmt.Sanitize(c.Folder))
	return nil
}
