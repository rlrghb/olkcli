package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailGetCmd struct {
	ID     string `arg:"" help:"Message ID"`
	Format string `help:"Output format: full|text|html" default:"full" enum:"full,text,html"`
}

func (c *MailGetCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	msg, err := client.GetMessage(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(msg, 1, "")
	}

	fmt.Printf("From:    %s\n", outfmt.Sanitize(msg.From))
	fmt.Printf("To:      %s\n", outfmt.Sanitize(strings.Join(msg.To, ", ")))
	fmt.Printf("Subject: %s\n", outfmt.Sanitize(msg.Subject))
	fmt.Printf("Date:    %s\n", outfmt.Sanitize(msg.ReceivedAt))
	fmt.Printf("Read:    %v\n", msg.IsRead)
	fmt.Println(strings.Repeat("-", 60))

	switch c.Format {
	case "text", "full":
		if msg.Body != "" {
			fmt.Println(outfmt.SanitizeMultiline(msg.Body))
		} else {
			fmt.Println(outfmt.SanitizeMultiline(msg.BodyPreview))
		}
	case "html":
		fmt.Println(outfmt.SanitizeMultiline(msg.Body))
	}

	return nil
}
