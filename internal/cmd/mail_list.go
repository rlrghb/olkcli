package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailListCmd struct {
	Folder  string  `help:"Mail folder ID or well-known name" short:"f" env:"OLK_MAIL_FOLDER"`
	Top     int32   `help:"Number of messages to return" default:"25" short:"n"`
	Unread  bool    `help:"Show only unread messages" short:"u"`
	From    string  `help:"Filter by sender email"`
	After   string  `help:"Filter messages after date (ISO 8601)"`
	Before  string  `help:"Filter messages before date (ISO 8601)"`
	Focused bool    `help:"Show only Focused Inbox messages"`
	Other   bool    `help:"Show only Other Inbox messages"`
	Order   *string `help:"Message order: newest|oldest (default newest)" enum:"newest,oldest"`
}

// mailListSelectableFields is the Graph selector set that ListMessages converts
// into mail-list JSON fields. Keep this limited to fields ListMessages converts into
// graphapi.MailMessage; accepting a Graph field that list output cannot render
// would otherwise produce a silently incomplete JSON object.
var mailListSelectableFields = map[string]bool{
	"id":               true,
	"subject":          true,
	"from":             true,
	"toRecipients":     true,
	"ccRecipients":     true,
	"bccRecipients":    true,
	"replyTo":          true,
	"receivedDateTime": true,
	"isRead":           true,
	"hasAttachments":   true,
	"bodyPreview":      true,
	"categories":       true,
	"conversationId":   true,
}

// unsupportedMailListSelectors are valid Graph message selectors that list
// output does not represent. Rejecting them makes the command's select
// contract honest instead of fetching fields that would disappear in output.
var unsupportedMailListSelectors = map[string]bool{
	"body":                 true,
	"importance":           true,
	"parentFolderId":       true,
	"sender":               true,
	"flag":                 true,
	"internetMessageId":    true,
	"createdDateTime":      true,
	"lastModifiedDateTime": true,
}

// mailListJSONMessage preserves MailMessage's output tags while allowing a
// selector to omit every unrequested field. Pointer fields retain selected
// zero values, whereas --concise can still zero tagged fields for omission.
type mailListJSONMessage struct {
	ID             *string   `json:"id,omitempty"`
	Subject        *string   `json:"subject,omitempty" untrusted:"true"`
	From           *string   `json:"from,omitempty" untrusted:"true"`
	To             *[]string `json:"to,omitempty" untrusted:"true"`
	Cc             *[]string `json:"cc,omitempty" untrusted:"true"`
	Bcc            *[]string `json:"bcc,omitempty" untrusted:"true"`
	ReplyTo        *[]string `json:"replyTo,omitempty" untrusted:"true"`
	ReceivedAt     *string   `json:"receivedDateTime,omitempty"`
	IsRead         *bool     `json:"isRead,omitempty"`
	HasAttachments *bool     `json:"hasAttachments,omitempty"`
	BodyPreview    *string   `json:"bodyPreview,omitempty" untrusted:"true" concise:"omit"`
	Categories     *[]string `json:"categories,omitempty"`
	ConversationID *string   `json:"conversationId,omitempty"`
}

func (c *MailListCmd) Run(ctx *RunContext) error {
	var selected []string
	var err error
	if ctx.Flags.JSON {
		selected, err = parseMailSelect(ctx.Flags.Select)
		if err != nil {
			return err
		}
	}

	if c.Focused && c.Other {
		return fmt.Errorf("cannot use both --focused and --other")
	}
	if (c.Focused || c.Other) && c.Order != nil {
		return fmt.Errorf("--order cannot be combined with --focused or --other")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
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

	order := ""
	if c.Order != nil {
		order = *c.Order
	}
	orderBy := mailListOrderBy(order)
	if c.Focused || c.Other {
		orderBy = ""
	}
	opts := graphapi.ListMessagesOptions{
		FolderID: c.Folder,
		Top:      c.Top,
		Filter:   filter,
		OrderBy:  orderBy,
		Select:   selected,
	}

	messages, err := client.ListMessages(ctx.Ctx, target, &opts)
	if err != nil {
		return err
	}
	if ctx.Flags.JSON {
		if len(selected) > 0 {
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

func parseMailSelect(selectFields SelectFields) ([]string, error) {
	if !selectFields.Set {
		return nil, nil
	}
	if selectFields.Value == "" {
		return nil, fmt.Errorf("--select cannot be empty")
	}
	selectValue := selectFields.Value
	fields := make([]string, 0, strings.Count(selectValue, ",")+1)
	seen := make(map[string]bool, cap(fields))
	for _, rawField := range strings.Split(selectValue, ",") {
		field := strings.TrimSpace(rawField)
		if field == "" {
			return nil, fmt.Errorf("--select cannot contain empty fields")
		}
		if seen[field] {
			return nil, fmt.Errorf("--select field %q is duplicated", field)
		}
		if _, ok := mailListSelectableFields[field]; !ok {
			if unsupportedMailListSelectors[field] {
				return nil, fmt.Errorf("--select field %q is not available in mail list output", field)
			}
			return nil, fmt.Errorf("invalid --select field %q", field)
		}
		seen[field] = true
		fields = append(fields, field)
	}
	return fields, nil
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

func projectMailMessages(messages []graphapi.MailMessage, selected []string) []mailListJSONMessage {
	projected := make([]mailListJSONMessage, len(messages))
	for i := range messages {
		message := &messages[i]
		projected[i] = projectMailMessage(message, selected)
	}
	return projected
}

func projectMailMessage(message *graphapi.MailMessage, selected []string) mailListJSONMessage {
	projected := mailListJSONMessage{}
	for _, field := range selected {
		switch field {
		case "id":
			projected.ID = &message.ID
		case "subject":
			projected.Subject = &message.Subject
		case "from":
			projected.From = &message.From
		case "toRecipients":
			projected.To = &message.To
		case "ccRecipients":
			projected.Cc = &message.Cc
		case "bccRecipients":
			projected.Bcc = &message.Bcc
		case "replyTo":
			projected.ReplyTo = &message.ReplyTo
		case "receivedDateTime":
			projected.ReceivedAt = &message.ReceivedAt
		case "isRead":
			projected.IsRead = &message.IsRead
		case "hasAttachments":
			projected.HasAttachments = &message.HasAttachments
		case "bodyPreview":
			projected.BodyPreview = &message.BodyPreview
		case "categories":
			projected.Categories = &message.Categories
		case "conversationId":
			projected.ConversationID = &message.ConversationID
		}
	}
	return projected
}
