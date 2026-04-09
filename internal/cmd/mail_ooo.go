package cmd

import (
	"fmt"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailOOOCmd is the parent command for out-of-office / auto-reply settings
type MailOOOCmd struct {
	Get MailOOOGetCmd `cmd:"" help:"Get auto-reply settings"`
	Set MailOOOSetCmd `cmd:"" help:"Set auto-reply"`
	Off MailOOOOffCmd `cmd:"" help:"Disable auto-reply"`
}

// MailOOOGetCmd retrieves the current auto-reply settings
type MailOOOGetCmd struct{}

func (c *MailOOOGetCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	settings, err := client.GetAutoReply(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(settings, 1, "")
	}

	fmt.Printf("Status:            %s\n", outfmt.Sanitize(settings.Status))
	fmt.Printf("External Audience: %s\n", outfmt.Sanitize(settings.ExternalAudience))
	if settings.InternalMessage != "" {
		fmt.Printf("Internal Message:  %s\n", outfmt.Sanitize(settings.InternalMessage))
	}
	if settings.ExternalMessage != "" {
		fmt.Printf("External Message:  %s\n", outfmt.Sanitize(settings.ExternalMessage))
	}
	if settings.StartTime != "" {
		fmt.Printf("Start:             %s\n", outfmt.Sanitize(settings.StartTime))
	}
	if settings.EndTime != "" {
		fmt.Printf("End:               %s\n", outfmt.Sanitize(settings.EndTime))
	}

	return nil
}

// MailOOOSetCmd enables auto-reply with the given message
type MailOOOSetCmd struct {
	Message         string `help:"Auto-reply message (internal)" required:"" short:"m"`
	ExternalMessage string `help:"Auto-reply message for external senders (defaults to --message)" short:"x"`
	Start           string `help:"Start date/time (ISO 8601, enables scheduled mode)"`
	End             string `help:"End date/time (ISO 8601, enables scheduled mode)"`
	Audience        string `help:"External audience: none|contactsOnly|all" default:"all" enum:"none,contactsOnly,all"`
}

func (c *MailOOOSetCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	externalMsg := c.ExternalMessage
	if externalMsg == "" {
		externalMsg = c.Message
	}

	status := "alwaysEnabled"
	var startStr, endStr string

	if c.Start != "" || c.End != "" {
		status = "scheduled"
		var startTime, endTime time.Time
		if c.Start != "" {
			t, err := parseOOOTime(c.Start)
			if err != nil {
				return fmt.Errorf("invalid --start: %w", err)
			}
			startTime = t
			startStr = t.UTC().Format("2006-01-02T15:04:05")
		}
		if c.End != "" {
			t, err := parseOOOTime(c.End)
			if err != nil {
				return fmt.Errorf("invalid --end: %w", err)
			}
			endTime = t
			endStr = t.UTC().Format("2006-01-02T15:04:05")
		}
		if c.Start != "" && c.End != "" && !endTime.After(startTime) {
			return fmt.Errorf("--end must be after --start")
		}
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would set auto-reply:\n  Status: %s\n  Message: %s\n", status, outfmt.Sanitize(c.Message))
		if status == "scheduled" {
			fmt.Printf("  Start: %s\n  End: %s\n", startStr, endStr)
		}
		return nil
	}

	err = client.SetAutoReply(ctx.Ctx, status, c.Message, externalMsg, startStr, endStr, c.Audience)
	if err != nil {
		return err
	}

	if status == "scheduled" {
		fmt.Println("Auto-reply enabled (scheduled).")
	} else {
		fmt.Println("Auto-reply enabled.")
	}
	return nil
}

// MailOOOOffCmd disables auto-reply
type MailOOOOffCmd struct{}

func (c *MailOOOOffCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.SetAutoReply(ctx.Ctx, "disabled", "", "", "", "", "")
	if err != nil {
		return err
	}

	fmt.Println("Auto-reply disabled.")
	return nil
}

func parseOOOTime(s string) (time.Time, error) {
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
