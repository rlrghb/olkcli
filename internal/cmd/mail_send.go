package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailSendCmd struct {
	To      []string `help:"Recipient email addresses" required:"" short:"t"`
	Subject string   `help:"Email subject" required:"" short:"s"`
	Body    string   `help:"Email body" short:"b"`
	CC      []string `help:"CC recipients"`
	BCC     []string `help:"BCC recipients"`
	HTML    bool     `help:"Send body as HTML"`
}

func (c *MailSendCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	body := c.Body
	// Read from stdin if no body provided
	if body == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 4<<20)) // 4 MB limit
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			body = strings.TrimSpace(string(data))
		}
	}

	for _, addr := range append(append(c.To, c.CC...), c.BCC...) {
		if err := graphapi.ValidateEmail(addr); err != nil {
			return err
		}
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would send email:\n  To: %s\n  Subject: %s\n  Body: %s\n", strings.Join(c.To, ", "), outfmt.Sanitize(c.Subject), outfmt.Sanitize(body))
		return nil
	}

	err = client.SendMessage(ctx.Ctx, c.Subject, body, c.To, c.CC, c.BCC, c.HTML)
	if err != nil {
		return err
	}

	fmt.Println("Message sent.")
	return nil
}
