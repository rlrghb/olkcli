package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailListCmd struct {
	Folder  string `help:"Mail folder ID or well-known name" short:"f" env:"OLK_MAIL_FOLDER"`
	Top     int32  `help:"Number of messages to return" default:"25" short:"n"`
	Unread  bool   `help:"Show only unread messages" short:"u"`
	From    string `help:"Filter by sender email"`
	After   string `help:"Filter messages after date (ISO 8601)"`
	Before  string `help:"Filter messages before date (ISO 8601)"`
	Focused bool   `help:"Show only Focused Inbox messages"`
	Other   bool   `help:"Show only Other Inbox messages"`
}

func (c *MailListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if c.Focused && c.Other {
		return fmt.Errorf("cannot use both --focused and --other")
	}

	filter, err := buildMailFilter(c.Unread, c.From, c.After, c.Before)
	if err != nil {
		return err
	}

	if c.Focused {
		if filter != "" {
			filter += " and "
		}
		filter += "inferenceClassification eq 'focused'"
	} else if c.Other {
		if filter != "" {
			filter += " and "
		}
		filter += "inferenceClassification eq 'other'"
	}

	opts := graphapi.ListMessagesOptions{
		FolderID: c.Folder,
		Top:      c.Top,
		Filter:   filter,
	}

	messages, err := client.ListMessages(ctx.Ctx, &opts)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(messages, len(messages), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"ID", "FROM", "SUBJECT", "DATE", "READ", "ATTACH"}
	var rows [][]string
	for i := range messages {
		m := &messages[i]
		read := " "
		if m.IsRead {
			read = "Y"
		}
		attach := ""
		if m.HasAttachments {
			attach = "Y"
		}
		subject := outfmt.Truncate(m.Subject, 60)
		date := outfmt.Truncate(outfmt.ConvertTime(m.ReceivedAt, loc), 16)
		id := outfmt.Truncate(m.ID, 15)
		rows = append(rows, []string{id, m.From, subject, date, read, attach})
	}

	return printer.Print(headers, rows, messages, len(messages), "")
}
