package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailCategorizeCmd sets categories on a message
type MailCategorizeCmd struct {
	ID         string   `arg:"" help:"Message ID"`
	Categories []string `help:"Category names" required:"" short:"c"`
}

func (c *MailCategorizeCmd) Run(ctx *RunContext) error {
	for _, cat := range c.Categories {
		if cat == "" {
			return fmt.Errorf("category name cannot be empty")
		}
		if len(cat) > 255 {
			return fmt.Errorf("category name too long (max 255 characters): %q", outfmt.Truncate(cat, 30))
		}
	}
	if len(c.Categories) > 25 {
		return fmt.Errorf("too many categories (max 25)")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would set categories on message %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.CategorizeMessage(ctx.Ctx, c.ID, c.Categories)
	if err != nil {
		return err
	}

	fmt.Println("Categories updated.")
	return nil
}
