package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type CalendarCmd struct {
	Events    CalendarEventsCmd    `cmd:"" help:"List calendar events"`
	Get       CalendarGetCmd       `cmd:"" help:"Get event details"`
	Create    CalendarCreateCmd    `cmd:"" help:"Create a calendar event"`
	Update    CalendarUpdateCmd    `cmd:"" help:"Update a calendar event"`
	Delete    CalendarDeleteCmd    `cmd:"" help:"Delete a calendar event"`
	Respond   CalendarRespondCmd   `cmd:"" help:"Respond to an event invitation"`
	Calendars    CalendarCalendarsCmd    `cmd:"" help:"List available calendars"`
	Availability CalendarAvailabilityCmd `cmd:"" help:"Check availability / free-busy"`
	View         CalendarViewCmd         `cmd:"" help:"Calendar view with expanded recurring events"`
	FindTimes    CalendarFindTimesCmd    `cmd:"" help:"Find available meeting times" name:"find-times"`
}

type CalendarEventsCmd struct {
	Days     int    `help:"Number of days to look ahead" default:"7" short:"d"`
	After    string `help:"Start date (ISO 8601)"`
	Before   string `help:"End date (ISO 8601)"`
	Calendar string `help:"Calendar ID"`
	Top      int32  `help:"Max events to return" default:"25" short:"n"`
}

func (c *CalendarEventsCmd) Run(ctx *RunContext) error {
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

	events, err := client.ListEvents(ctx.Ctx, start, end, c.Calendar, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(events, len(events), "")
	}

	headers := []string{"ID", "SUBJECT", "START", "END", "LOCATION", "STATUS", "RECURRENCE"}
	var rows [][]string
	for _, e := range events {
		id := outfmt.Truncate(e.ID, 15)
		subject := outfmt.Truncate(e.Subject, 40)
		startStr := outfmt.Truncate(e.Start, 16)
		endStr := outfmt.Truncate(e.End, 16)
		rows = append(rows, []string{id, subject, startStr, endStr, e.Location, e.Status, e.Recurrence})
	}

	return printer.Print(headers, rows, events, len(events), "")
}

type CalendarGetCmd struct {
	ID string `arg:"" help:"Event ID"`
}

func (c *CalendarGetCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	event, err := client.GetEvent(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(event, 1, "")
	}

	fmt.Printf("Subject:   %s\n", outfmt.Sanitize(event.Subject))
	fmt.Printf("Start:     %s\n", outfmt.Sanitize(event.Start))
	fmt.Printf("End:       %s\n", outfmt.Sanitize(event.End))
	fmt.Printf("Location:  %s\n", outfmt.Sanitize(event.Location))
	fmt.Printf("Organizer: %s\n", outfmt.Sanitize(event.Organizer))
	fmt.Printf("Status:    %s\n", outfmt.Sanitize(event.Status))
	if event.Recurrence != "" {
		fmt.Printf("Recurrence: %s\n", outfmt.Sanitize(event.Recurrence))
	}
	fmt.Printf("All Day:   %v\n", event.IsAllDay)
	fmt.Printf("Online:    %v\n", event.IsOnline)
	if event.OnlineURL != "" {
		fmt.Printf("Meeting URL: %s\n", outfmt.Sanitize(event.OnlineURL))
	}
	if len(event.Attendees) > 0 {
		fmt.Printf("Attendees: %s\n", outfmt.Sanitize(strings.Join(event.Attendees, ", ")))
	}
	if event.Body != "" {
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(outfmt.SanitizeMultiline(event.Body))
	}

	return nil
}

type CalendarCreateCmd struct {
	Subject       string   `help:"Event subject" required:"" short:"s"`
	Start         string   `help:"Start time (ISO 8601)" required:""`
	End           string   `help:"End time (ISO 8601)" required:""`
	Location      string   `help:"Event location" short:"l"`
	Attendees     []string `help:"Attendee email addresses" short:"a"`
	AllDay        bool     `help:"All-day event"`
	OnlineMeeting bool    `help:"Create online meeting"`
	Recurrence    string   `help:"Recurrence: daily|weekdays|weekly|monthly|yearly" short:"r"`
}

func (c *CalendarCreateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	start, err := parseTime(c.Start)
	if err != nil {
		return fmt.Errorf("invalid --start: %w", err)
	}
	end, err := parseTime(c.End)
	if err != nil {
		return fmt.Errorf("invalid --end: %w", err)
	}

	if !end.After(start) {
		return fmt.Errorf("--end must be after --start")
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create event:\n  Subject: %s\n  Start: %s\n  End: %s\n", outfmt.Sanitize(c.Subject), c.Start, c.End)
		return nil
	}

	event, err := client.CreateEvent(ctx.Ctx, c.Subject, start, end, c.Location, c.Attendees, c.AllDay, c.OnlineMeeting, c.Recurrence)
	if err != nil {
		return err
	}

	fmt.Printf("Event created: %s (ID: %s)\n", outfmt.Sanitize(event.Subject), event.ID)
	return nil
}

type CalendarUpdateCmd struct {
	ID       string `arg:"" help:"Event ID"`
	Subject  string `help:"New subject" short:"s"`
	Start    string `help:"New start time (ISO 8601)"`
	End      string `help:"New end time (ISO 8601)"`
	Location string `help:"New location" short:"l"`
}

func (c *CalendarUpdateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	var subject, location *string
	var start, end *time.Time

	if c.Subject != "" {
		subject = &c.Subject
	}
	if c.Location != "" {
		location = &c.Location
	}
	if c.Start != "" {
		t, err := parseTime(c.Start)
		if err != nil {
			return fmt.Errorf("invalid --start: %w", err)
		}
		start = &t
	}
	if c.End != "" {
		t, err := parseTime(c.End)
		if err != nil {
			return fmt.Errorf("invalid --end: %w", err)
		}
		end = &t
	}

	event, err := client.UpdateEvent(ctx.Ctx, c.ID, subject, start, end, location)
	if err != nil {
		return err
	}

	fmt.Printf("Event updated: %s\n", outfmt.Sanitize(event.Subject))
	return nil
}

type CalendarDeleteCmd struct {
	ID string `arg:"" help:"Event ID"`
}

func (c *CalendarDeleteCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete event %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	err = client.DeleteEvent(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Event deleted.")
	return nil
}

type CalendarRespondCmd struct {
	ID       string `arg:"" help:"Event ID"`
	Response string `arg:"" help:"Response: accept|decline|tentative" enum:"accept,decline,tentative"`
}

func (c *CalendarRespondCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.RespondToEvent(ctx.Ctx, c.ID, c.Response)
	if err != nil {
		return err
	}

	fmt.Printf("Responded '%s' to event.\n", c.Response)
	return nil
}

type CalendarCalendarsCmd struct{}

func (c *CalendarCalendarsCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	calendars, err := client.ListCalendars(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(calendars, len(calendars), "")
	}

	headers := []string{"ID", "NAME", "COLOR", "OWNER"}
	var rows [][]string
	for _, cal := range calendars {
		rows = append(rows, []string{cal.ID, cal.Name, cal.Color, cal.Owner})
	}

	return printer.Print(headers, rows, calendars, len(calendars), "")
}

func parseTime(s string) (time.Time, error) {
	// Try RFC3339 first, then date-only
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse("2006-01-02T15:04", s)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as time (try RFC3339 or 2006-01-02T15:04 format)", s)
}
