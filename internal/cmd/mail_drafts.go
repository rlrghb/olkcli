package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailDraftsCmd groups draft-related subcommands
type MailDraftsCmd struct {
	List   MailDraftsListCmd   `cmd:"" help:"List draft messages"`
	Create MailDraftsCreateCmd `cmd:"" help:"Create a draft message"`
	Send   MailDraftsSendCmd   `cmd:"" help:"Send a draft message"`
	Delete MailDraftsDeleteCmd `cmd:"" help:"Delete a draft message"`
}

// MailDraftsListCmd lists draft messages
type MailDraftsListCmd struct {
	Top int32 `help:"Number of drafts to return" default:"25" short:"n"`
}

func (c *MailDraftsListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	drafts, err := client.ListDrafts(ctx.Ctx, target, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(drafts, len(drafts), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"ID", "SUBJECT", "TO", "CREATED"}
	rows := make([][]string, 0, len(drafts))
	for _, d := range drafts {
		id := outfmt.Truncate(d.ID, 15)
		subject := outfmt.Truncate(d.Subject, 60)
		to := outfmt.Truncate(strings.Join(d.To, ", "), 40)
		created := outfmt.Truncate(outfmt.ConvertTime(d.Created, loc), 16)
		rows = append(rows, []string{id, subject, to, created})
	}

	return printer.Print(headers, rows, drafts, len(drafts), "")
}

// MailDraftsCreateCmd creates a new draft message
type MailDraftsCreateCmd struct {
	To      []string `help:"Recipient email addresses" required:"" short:"t"`
	Subject string   `help:"Email subject" required:"" short:"s"`
	Body    string   `help:"Email body" short:"b"`
	CC      []string `help:"CC recipients"`
	BCC     []string `help:"BCC recipients"`
	HTML    bool     `help:"Body is HTML"`
}

func (c *MailDraftsCreateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	body := c.Body
	// Read from stdin if no body provided
	if body == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20)) // 4 MB limit
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			body = strings.TrimSpace(string(data))
		}
	}

	if len(body) > maxBodySize {
		return fmt.Errorf("draft body exceeds maximum size of 4MB")
	}

	for _, addr := range append(append(c.To, c.CC...), c.BCC...) {
		if err := graphapi.ValidateEmail(addr); err != nil {
			return err
		}
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create draft:\n  In: %s\n  To: %s\n  Subject: %s\n  Body: %s\n",
			describeMailbox(target), strings.Join(c.To, ", "), outfmt.Sanitize(c.Subject), outfmt.Sanitize(body))
		return nil
	}

	draft, err := client.CreateDraft(ctx.Ctx, target, c.Subject, body, c.To, c.CC, c.BCC, c.HTML)
	if err != nil {
		return err
	}

	fmt.Printf("Draft created in %s: %s (ID: %s)\n", describeMailbox(target), outfmt.Sanitize(draft.Subject), outfmt.Sanitize(draft.ID))
	return nil
}

// MailDraftsSendCmd sends an existing draft
type MailDraftsSendCmd struct {
	ID string `arg:"" help:"Draft message ID"`
}

func (c *MailDraftsSendCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would send draft %s from %s\n", outfmt.Sanitize(c.ID), describeMailbox(target))
		return nil
	}

	err = client.SendDraft(ctx.Ctx, target, c.ID)
	if err != nil {
		return err
	}

	fmt.Printf("Draft sent from %s.\n", describeMailbox(target))
	return nil
}

// MailDraftsDeleteCmd deletes a draft message
type MailDraftsDeleteCmd struct {
	ID string `arg:"" help:"Draft message ID"`
}

func (c *MailDraftsDeleteCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete draft %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete draft %s from %s\n", outfmt.Sanitize(c.ID), describeMailbox(target))
		return nil
	}

	err = client.DeleteDraft(ctx.Ctx, target, c.ID)
	if err != nil {
		return err
	}

	fmt.Printf("Draft deleted from %s.\n", describeMailbox(target))
	return nil
}

// describeMailbox names the mailbox an operation acts on, so that output and dry
// runs distinguish your own Drafts folder from a shared one. A draft left in the
// wrong mailbox is invisible to the person waiting for it.
func describeMailbox(target string) string {
	if target == "" {
		return ownMailboxLabel
	}
	return target
}
