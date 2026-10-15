package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailSendCmd struct {
	To          []string `help:"Recipient email addresses" required:"" short:"t"`
	Subject     string   `help:"Email subject" required:"" short:"s"`
	Body        string   `help:"Email body" short:"b"`
	CC          []string `help:"CC recipients"`
	BCC         []string `help:"BCC recipients"`
	HTML        bool     `help:"Send body as HTML"`
	Attach      []string `help:"File paths to attach" type:"path"`
	Importance  string   `help:"Message importance: low|normal|high" enum:",low,normal,high" default:""`
	ReadReceipt bool     `help:"Request a read receipt"`
}

const (
	// maxAttachmentTotal is the Graph API limit for total attachment size (35 MB).
	maxAttachmentTotal = 35 << 20
	// maxBodySize is the maximum email body size (4 MB).
	maxBodySize = 4 << 20
)

func (c *MailSendCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	body, err := ctx.bodyOrStdin(c.Body)
	if err != nil {
		return err
	}

	if len(body) > maxBodySize {
		return fmt.Errorf("email body exceeds maximum size of 4MB")
	}

	for _, addr := range append(append(c.To, c.CC...), c.BCC...) {
		if err := graphapi.ValidateEmail(addr); err != nil {
			return err
		}
	}

	// Input is checked before signing in, so a bad call fails fast.
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Process attachments
	var attachments []graphapi.AttachmentInput
	if len(c.Attach) > 0 {
		var totalSize int64
		for _, path := range c.Attach {
			// Each file may use what the earlier ones left of the limit.
			data, err := readLocalFile(path, maxAttachmentTotal-totalSize)
			if err != nil {
				return fmt.Errorf("attachment %q: %w", path, err)
			}

			totalSize += int64(len(data))
			if totalSize > maxAttachmentTotal {
				return fmt.Errorf("total attachment size exceeds 35MB Graph API limit")
			}

			contentType := http.DetectContentType(data)
			attachments = append(attachments, graphapi.AttachmentInput{
				Name:        filepath.Base(path),
				ContentType: contentType,
				Content:     data,
			})
		}
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would send email:\n  From: %s\n  To: %s\n", describeMailbox(target), strings.Join(c.To, ", "))
		if len(c.CC) > 0 {
			fmt.Printf("  Cc: %s\n", strings.Join(c.CC, ", "))
		}
		if len(c.BCC) > 0 {
			fmt.Printf("  Bcc: %s\n", strings.Join(c.BCC, ", "))
		}
		fmt.Printf("  Subject: %s\n  Body: %s\n", outfmt.Sanitize(c.Subject), outfmt.Sanitize(body))
		if len(attachments) > 0 {
			fmt.Printf("  Attachments: %d file(s)\n", len(attachments))
		}
		return nil
	}

	err = client.SendMessage(ctx.Ctx, target, &graphapi.SendMessageOptions{
		Subject:     c.Subject,
		Body:        body,
		To:          c.To,
		CC:          c.CC,
		BCC:         c.BCC,
		IsHTML:      c.HTML,
		Attachments: attachments,
		Importance:  c.Importance,
		ReadReceipt: c.ReadReceipt,
	})
	if err != nil {
		return err
	}

	if target != "" {
		fmt.Printf("Message sent from %s.\n", target)
		return nil
	}
	fmt.Println("Message sent.")
	return nil
}
