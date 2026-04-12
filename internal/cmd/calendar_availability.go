package cmd

import (
	"fmt"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// CalendarAvailabilityCmd checks free/busy availability for one or more users
type CalendarAvailabilityCmd struct {
	Emails []string `help:"Email addresses to check" required:"" short:"e"`
	Days   int      `help:"Days to look ahead" default:"1" short:"d"`
	After  string   `help:"Start date (ISO 8601)"`
	Before string   `help:"End date (ISO 8601)"`
}

func (c *CalendarAvailabilityCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	days := c.Days
	if days <= 0 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	start := time.Now()
	end := start.AddDate(0, 0, days)

	if c.After != "" {
		t, err := time.Parse(time.RFC3339, c.After)
		if err != nil {
			t, err = time.Parse("2006-01-02", c.After)
			if err != nil {
				return fmt.Errorf("invalid --after date: %w", err)
			}
		}
		start = t
	}
	if c.Before != "" {
		t, err := time.Parse(time.RFC3339, c.Before)
		if err != nil {
			t, err = time.Parse("2006-01-02", c.Before)
			if err != nil {
				return fmt.Errorf("invalid --before date: %w", err)
			}
		}
		end = t
	}

	schedules, err := client.GetSchedule(ctx.Ctx, c.Emails, start, end)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(schedules, len(schedules), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"EMAIL", "STATUS", "START", "END", "SUBJECT"}
	var rows [][]string
	for _, sched := range schedules {
		if len(sched.Items) == 0 {
			rows = append(rows, []string{sched.Email, "free", "", "", ""})
			continue
		}
		for _, item := range sched.Items {
			rows = append(rows, []string{
				sched.Email,
				item.Status,
				outfmt.Truncate(outfmt.ConvertTime(item.Start, loc), 16),
				outfmt.Truncate(outfmt.ConvertTime(item.End, loc), 16),
				outfmt.Truncate(item.Subject, 40),
			})
		}
	}

	return printer.Print(headers, rows, schedules, len(schedules), "")
}
