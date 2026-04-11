package cmd

import (
	"fmt"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type CalendarViewCmd struct {
	Days     int    `help:"Number of days to show" default:"7" short:"d"`
	After    string `help:"Start date (ISO 8601)"`
	Before   string `help:"End date (ISO 8601)"`
	Calendar string `help:"Calendar ID"`
	Top      int32  `help:"Max events to return" default:"50" short:"n"`
}

func (c *CalendarViewCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	days := c.Days
	if days <= 0 {
		days = 7
	}
	if days > 365 {
		days = 365
	}
	start := time.Now()
	end := start.AddDate(0, 0, days)

	if c.After != "" {
		t, err := parseTime(c.After)
		if err != nil {
			return fmt.Errorf("invalid --after date: %w", err)
		}
		start = t
	}
	if c.Before != "" {
		t, err := parseTime(c.Before)
		if err != nil {
			return fmt.Errorf("invalid --before date: %w", err)
		}
		end = t
	}

	events, err := client.ListCalendarView(ctx.Ctx, start, end, c.Calendar, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(events, len(events), "")
	}

	headers := []string{"ID", "SUBJECT", "START", "END", "LOCATION", "STATUS", "RECURRENCE"}
	var rows [][]string
	for i := range events {
		e := &events[i]
		id := outfmt.Truncate(e.ID, 15)
		subject := outfmt.Truncate(e.Subject, 40)
		startStr := outfmt.Truncate(e.Start, 16)
		endStr := outfmt.Truncate(e.End, 16)
		rows = append(rows, []string{id, subject, startStr, endStr, outfmt.Sanitize(e.Location), e.Status, e.Recurrence})
	}

	return printer.Print(headers, rows, events, len(events), "")
}
