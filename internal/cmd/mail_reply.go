package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailReplyCmd struct {
	ID       string `arg:"" help:"Message ID to reply to"`
	Body     string `help:"Reply body" required:"" short:"b"`
	ReplyAll bool   `help:"Reply to all recipients" short:"a"`
}

func (c *MailReplyCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	action := "reply"
	if c.ReplyAll {
		action = "reply-all"
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would %s to message %s as %s\n", action, outfmt.Sanitize(c.ID), describeMailbox(target))
		return nil
	}

	if err := client.ReplyMessage(ctx.Ctx, target, c.ID, c.Body, c.ReplyAll); err != nil {
		return err
	}

	if target != "" {
		fmt.Printf("Reply sent from %s.\n", target)
		return nil
	}
	if c.ReplyAll {
		fmt.Println("Reply-all sent.")
	} else {
		fmt.Println("Reply sent.")
	}
	return nil
}
