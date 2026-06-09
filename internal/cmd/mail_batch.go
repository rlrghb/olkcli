package cmd

import "fmt"

// MailBatchCmd fetches up to 20 messages by ID in one Graph $batch round-trip.
type MailBatchCmd struct {
	ID []string `help:"Message ID to fetch (repeatable, max 20)" name:"id"`
}

func (c *MailBatchCmd) Run(ctx *RunContext) error {
	if len(c.ID) == 0 {
		return fmt.Errorf("provide at least one --id")
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	messages, err := client.GetMessagesBatch(ctx.Ctx, target, c.ID)
	if err != nil {
		return err
	}
	return printMessageList(ctx, messages)
}
