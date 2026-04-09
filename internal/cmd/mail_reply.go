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

	if ctx.Flags.DryRun {
		action := "reply"
		if c.ReplyAll {
			action = "reply-all"
		}
		fmt.Printf("Would %s to message %s\n", action, outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.ReplyMessage(ctx.Ctx, c.ID, c.Body, c.ReplyAll)
	if err != nil {
		return err
	}

	if c.ReplyAll {
		fmt.Println("Reply-all sent.")
	} else {
		fmt.Println("Reply sent.")
	}
	return nil
}
