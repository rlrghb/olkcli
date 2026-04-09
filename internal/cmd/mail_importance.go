package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailImportanceCmd sets the importance level on a message
type MailImportanceCmd struct {
	ID         string `arg:"" help:"Message ID"`
	Importance string `arg:"" help:"Importance: low|normal|high" enum:"low,normal,high"`
}

func (c *MailImportanceCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would set importance of message %s to %s\n", outfmt.Sanitize(c.ID), outfmt.Sanitize(c.Importance))
		return nil
	}

	err = client.SetImportance(ctx.Ctx, c.ID, c.Importance)
	if err != nil {
		return err
	}

	fmt.Printf("Message importance set to %s.\n", outfmt.Sanitize(c.Importance))
	return nil
}
