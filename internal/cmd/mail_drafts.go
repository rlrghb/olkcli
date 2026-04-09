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

	drafts, err := client.ListDrafts(ctx.Ctx, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(drafts, len(drafts), "")
	}

	headers := []string{"ID", "SUBJECT", "TO", "CREATED"}
	var rows [][]string
	for _, d := range drafts {
		id := outfmt.Truncate(d.ID, 15)
		subject := outfmt.Truncate(d.Subject, 60)
		to := outfmt.Truncate(strings.Join(d.To, ", "), 40)
		created := outfmt.Truncate(d.Created, 16)
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
		fmt.Printf("Would create draft:\n  To: %s\n  Subject: %s\n  Body: %s\n",
			strings.Join(c.To, ", "), outfmt.Sanitize(c.Subject), outfmt.Sanitize(body))
		return nil
	}

	draft, err := client.CreateDraft(ctx.Ctx, c.Subject, body, c.To, c.CC, c.BCC, c.HTML)
	if err != nil {
		return err
	}

	fmt.Printf("Draft created: %s (ID: %s)\n", outfmt.Sanitize(draft.Subject), outfmt.Sanitize(draft.ID))
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

	if ctx.Flags.DryRun {
		fmt.Printf("Would send draft %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.SendDraft(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Draft sent.")
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

	if !ctx.Flags.Force {
		return fmt.Errorf("delete draft %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete draft %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.DeleteDraft(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Draft deleted.")
	return nil
}
