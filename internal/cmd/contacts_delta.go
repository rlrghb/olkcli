package cmd

import (
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// ContactsDeltaCmd performs an incremental sync of contacts via Graph's delta
// endpoint. Omit --token for a fresh sync; pass the returned token next time.
type ContactsDeltaCmd struct {
	Token string `help:"Delta token from a previous call (omit to start a fresh sync)"`
	Top   int32  `help:"Max items per page" short:"n"`
}

func (c *ContactsDeltaCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	items, page, err := client.DeltaContacts(ctx.Ctx, target, c.Token, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	headers := []string{"ID", "CHANGE", "NAME", "EMAILS"}
	rows := make([][]string, 0, len(items))
	for i := range items {
		ct := &items[i]
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(ct.ID), 15),
			changeLabel(ct.Removed),
			outfmt.Truncate(outfmt.Sanitize(ct.DisplayName), 30),
			outfmt.Truncate(outfmt.Sanitize(strings.Join(ct.Emails, ", ")), 40),
		})
	}
	return printer.PrintDelta(headers, rows, items, len(items), page.Token, page.Complete)
}
