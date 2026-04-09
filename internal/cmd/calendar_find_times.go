package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type CalendarFindTimesCmd struct {
	Attendees []string `help:"Attendee email addresses" required:"" short:"a"`
	Duration  int32    `help:"Meeting duration in minutes" default:"60" short:"d"`
	After     string   `help:"Search after date (ISO 8601)"`
	Before    string   `help:"Search before date (ISO 8601)"`
}

func (c *CalendarFindTimesCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	var start, end time.Time
	if c.After != "" {
		start, err = parseTime(c.After)
		if err != nil {
			return fmt.Errorf("invalid --after: %w", err)
		}
	} else {
		start = time.Now()
	}
	if c.Before != "" {
		end, err = parseTime(c.Before)
		if err != nil {
			return fmt.Errorf("invalid --before: %w", err)
		}
	} else {
		end = start.AddDate(0, 0, 7)
	}

	duration := c.Duration
	if duration <= 0 {
		duration = 60
	}
	if duration > 1440 {
		return fmt.Errorf("--duration cannot exceed 1440 minutes (24 hours)")
	}

	suggestions, err := client.FindMeetingTimes(ctx.Ctx, c.Attendees, start, end, duration)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(suggestions, len(suggestions), "")
	}

	headers := []string{"START", "END", "CONFIDENCE", "ATTENDEE AVAILABILITY"}
	var rows [][]string
	for _, s := range suggestions {
		var avails []string
		for _, a := range s.AttendeeAvailabilities {
			avails = append(avails, fmt.Sprintf("%s:%s", a.Email, a.Availability))
		}
		rows = append(rows, []string{
			outfmt.Truncate(s.Start, 16),
			outfmt.Truncate(s.End, 16),
			fmt.Sprintf("%.0f%%", s.Confidence*100),
			strings.Join(avails, ", "),
		})
	}

	return printer.Print(headers, rows, suggestions, len(suggestions), "")
}
