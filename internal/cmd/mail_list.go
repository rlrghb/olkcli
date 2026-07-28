package cmd

import (
	"fmt"
	"reflect"
	"strings"

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
	Order   string `help:"Message order: newest|oldest" default:"newest" enum:"newest,oldest"`
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

	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	opts := graphapi.ListMessagesOptions{
		FolderID: c.Folder,
		Top:      c.Top,
		Filter:   filter,
		OrderBy:  mailListOrderBy(c.Order),
		Select:   parseMailSelect(ctx.Flags.Select),
	}

	messages, err := client.ListMessages(ctx.Ctx, target, &opts)
	if err != nil {
		return err
	}
	if ctx.Flags.JSON {
		if selected := parseMailSelect(ctx.Flags.Select); len(selected) > 0 {
			return ctx.Printer().PrintJSON(projectMailMessages(messages, selected), len(messages), "")
		}
	}

	return printMessageList(ctx, messages)
}

func mailListOrderBy(order string) string {
	if order == "oldest" {
		return "receivedDateTime asc"
	}
	return "receivedDateTime desc"
}

func parseMailSelect(selectFields string) []string {
	if selectFields == "" {
		return nil
	}
	fields := strings.Split(selectFields, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

// printMessageList renders a slice of messages as JSON (full structs) or an
// aligned table, shared by mail list, mail batch, and mail thread.
func printMessageList(ctx *RunContext, messages []graphapi.MailMessage) error {
	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(messages, len(messages), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"ID", "FROM", "SUBJECT", "DATE", "READ", "ATTACH"}
	rows := make([][]string, 0, len(messages))
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

func projectMailMessages(messages []graphapi.MailMessage, selected []string) []map[string]any {
	projected := make([]map[string]any, len(messages))
	messageType := reflect.TypeOf(graphapi.MailMessage{})
	for i := range messages {
		projected[i] = map[string]any{}
		messageValue := reflect.ValueOf(messages[i])
		for fieldIndex := range messageType.NumField() {
			field := messageType.Field(fieldIndex)
			jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
			if jsonName == "" || !mailJSONFieldSelected(jsonName, selected) {
				continue
			}
			projected[i][jsonName] = messageValue.Field(fieldIndex).Interface()
		}
	}
	return projected
}

func mailJSONFieldSelected(jsonName string, selected []string) bool {
	for _, field := range selected {
		if field == jsonName || (field == "toRecipients" && jsonName == "to") {
			return true
		}
	}
	return false
}
