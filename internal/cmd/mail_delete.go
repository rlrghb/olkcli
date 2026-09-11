package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailDeleteCmd struct {
	ID string `arg:"" help:"Message ID"`
}

func (c *MailDeleteCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete message %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	if ctx.Flags.DryRun {
		if target == "" {
			fmt.Printf("Would delete message %s\n", outfmt.Sanitize(c.ID))
		} else {
			fmt.Printf("Would delete message %s from %s\n", outfmt.Sanitize(c.ID), describeMailbox(target))
		}
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if err := client.DeleteMessage(ctx.Ctx, target, c.ID); err != nil {
		return err
	}

	if target == "" {
		fmt.Println("Message deleted.")
	} else {
		fmt.Printf("Message deleted from %s.\n", describeMailbox(target))
	}
	return nil
}
