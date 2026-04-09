package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailDeleteCmd struct {
	ID string `arg:"" help:"Message ID"`
}

func (c *MailDeleteCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete message %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete message %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.DeleteMessage(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Message deleted.")
	return nil
}
