package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailMarkCmd struct {
	ID     string `arg:"" help:"Message ID"`
	Read   bool   `help:"Mark as read" xor:"state"`
	Unread bool   `help:"Mark as unread" xor:"state"`
}

func (c *MailMarkCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	if !c.Read && !c.Unread {
		return fmt.Errorf("specify --read or --unread")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	state := "unread"
	if c.Read {
		state = "read"
	}
	if ctx.Flags.DryRun {
		fmt.Printf("Would mark message %s as %s%s\n", outfmt.Sanitize(c.ID), state, mailboxSuffix("in", target))
		return nil
	}

	err = client.MarkMessage(ctx.Ctx, target, c.ID, c.Read)
	if err != nil {
		return err
	}

	fmt.Printf("Marked as %s.\n", state)
	return nil
}
