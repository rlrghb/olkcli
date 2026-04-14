package cmd

import "github.com/rlrghb/olkcli/internal/outfmt"

type MailSearchCmd struct {
	Query string `arg:"" help:"Search query — supports KQL operators (from:, subject:, hasAttachment:, etc.)"`
	Top   int32  `help:"Number of results" default:"25" short:"n"`
}

func (c *MailSearchCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	messages, err := client.SearchMessages(ctx.Ctx, c.Query, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(messages, len(messages), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"ID", "FROM", "SUBJECT", "DATE"}
	var rows [][]string
	for i := range messages {
		m := &messages[i]
		id := outfmt.Truncate(m.ID, 15)
		date := outfmt.Truncate(outfmt.ConvertTime(m.ReceivedAt, loc), 16)
		subject := outfmt.Truncate(m.Subject, 60)
		rows = append(rows, []string{id, m.From, subject, date})
	}

	return printer.Print(headers, rows, messages, len(messages), "")
}
