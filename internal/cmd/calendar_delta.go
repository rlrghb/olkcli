package cmd

import (
	"fmt"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// CalendarDeltaCmd performs an incremental sync of calendar events via Graph's
// calendarView delta endpoint. The window (--days/--after/--before) applies to
// the initial sync only; on resume the token already encodes it.
type CalendarDeltaCmd struct {
	Days   int    `help:"Window size in days ahead for the initial sync" default:"30" short:"d"`
	After  string `help:"Window start (ISO 8601); overrides --days start"`
	Before string `help:"Window end (ISO 8601); overrides --days end"`
	Token  string `help:"Delta token from a previous call (omit to start a fresh sync)"`
	Top    int32  `help:"Max items per page" short:"n"`
}

func (c *CalendarDeltaCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	days := c.Days
	if days <= 0 {
		days = 30
	}
	start := time.Now()
	end := start.AddDate(0, 0, days)
	if c.After != "" {
		if start, err = parseTime(c.After); err != nil {
			return fmt.Errorf("invalid --after: %w", err)
		}
	}
	if c.Before != "" {
		if end, err = parseTime(c.Before); err != nil {
			return fmt.Errorf("invalid --before: %w", err)
		}
	}

	items, page, err := client.DeltaCalendarView(ctx.Ctx, target, c.Token, start, end, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	loc, _ := ctx.Timezone()
	headers := []string{"ID", "CHANGE", "SUBJECT", "START", "END", "LOCATION"}
	rows := make([][]string, 0, len(items))
	for i := range items {
		e := &items[i]
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(e.ID), 15),
			changeLabel(e.Removed),
			outfmt.Truncate(outfmt.Sanitize(e.Subject), 40),
			outfmt.Truncate(outfmt.Sanitize(outfmt.ConvertTime(e.Start, loc)), 16),
			outfmt.Truncate(outfmt.Sanitize(outfmt.ConvertTime(e.End, loc)), 16),
			outfmt.Truncate(outfmt.Sanitize(e.Location), 20),
		})
	}
	return printer.PrintDelta(headers, rows, items, len(items), page.Token, page.Complete)
}
