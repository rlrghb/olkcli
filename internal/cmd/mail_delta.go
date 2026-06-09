package cmd

import "github.com/rlrghb/olkcli/internal/outfmt"

// MailDeltaCmd performs an incremental sync of a mail folder via Graph's delta
// endpoint. Omit --token for a fresh sync; pass the returned token next time to
// get only what changed since.
type MailDeltaCmd struct {
	Folder string `help:"Mail folder ID or well-known name" short:"f" default:"inbox"`
	Token  string `help:"Delta token from a previous call (omit to start a fresh sync)"`
	Top    int32  `help:"Max items per page" short:"n"`
}

func (c *MailDeltaCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	items, page, err := client.DeltaMessages(ctx.Ctx, target, c.Folder, c.Token, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	loc, _ := ctx.Timezone()
	headers := []string{"ID", "CHANGE", "FROM", "SUBJECT", "DATE"}
	rows := make([][]string, 0, len(items))
	for i := range items {
		m := &items[i]
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(m.ID), 15),
			changeLabel(m.Removed),
			outfmt.Truncate(outfmt.Sanitize(m.From), 25),
			outfmt.Truncate(outfmt.Sanitize(m.Subject), 40),
			outfmt.Truncate(outfmt.Sanitize(outfmt.ConvertTime(m.ReceivedAt, loc)), 16),
		})
	}
	return printer.PrintDelta(headers, rows, items, len(items), page.Token, page.Complete)
}

// changeLabel renders a delta item's change kind for table output.
func changeLabel(removed bool) string {
	if removed {
		return "removed"
	}
	return "upsert"
}
