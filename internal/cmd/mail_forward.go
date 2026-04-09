package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailForwardCmd struct {
	ID      string   `arg:"" help:"Message ID to forward"`
	To      []string `help:"Recipient email addresses" required:"" short:"t"`
	Comment string   `help:"Comment to include" short:"c"`
}

func (c *MailForwardCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	for _, addr := range c.To {
		if err := graphapi.ValidateEmail(addr); err != nil {
			return err
		}
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would forward message %s to %s\n", outfmt.Sanitize(c.ID), strings.Join(c.To, ", "))
		return nil
	}

	err = client.ForwardMessage(ctx.Ctx, c.ID, c.Comment, c.To)
	if err != nil {
		return err
	}

	fmt.Println("Message forwarded.")
	return nil
}
