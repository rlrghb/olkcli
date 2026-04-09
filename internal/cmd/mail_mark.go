package cmd

import "fmt"

type MailMarkCmd struct {
	ID     string `arg:"" help:"Message ID"`
	Read   bool   `help:"Mark as read" xor:"state"`
	Unread bool   `help:"Mark as unread" xor:"state"`
}

func (c *MailMarkCmd) Run(ctx *RunContext) error {
	if !c.Read && !c.Unread {
		return fmt.Errorf("specify --read or --unread")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.MarkMessage(ctx.Ctx, c.ID, c.Read)
	if err != nil {
		return err
	}

	if c.Read {
		fmt.Println("Marked as read.")
	} else {
		fmt.Println("Marked as unread.")
	}
	return nil
}
