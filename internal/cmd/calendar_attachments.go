package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

const (
	calendarAttachmentIDHeader   = "ID"
	calendarAttachmentNameHeader = "NAME"
	calendarAttachmentTypeHeader = "TYPE"
	calendarAttachmentSizeHeader = "SIZE"
)

// CalendarAttachmentsCmd manages file attachments on calendar events.
type CalendarAttachmentsCmd struct {
	List     CalendarAttachmentsListCmd     `cmd:"" help:"List event attachments"`
	Add      CalendarAttachmentsAddCmd      `cmd:"" help:"Add a file attachment"`
	Download CalendarAttachmentsDownloadCmd `cmd:"" help:"Download an attachment"`
	Delete   CalendarAttachmentsDeleteCmd   `cmd:"" help:"Delete an attachment"`
}

// CalendarAttachmentsListCmd lists attachments on an event.
type CalendarAttachmentsListCmd struct {
	EventID string `arg:"" help:"Event ID"`
}

func (c *CalendarAttachmentsListCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachments, err := client.ListCalendarAttachments(ctx.Ctx, target, c.EventID)
	if err != nil {
		return err
	}
	if len(attachments) == 0 {
		fmt.Println("No attachments.")
		return nil
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(attachments, len(attachments), "")
	}

	headers := []string{calendarAttachmentIDHeader, calendarAttachmentNameHeader, calendarAttachmentTypeHeader, calendarAttachmentSizeHeader}
	rows := make([][]string, 0, len(attachments))
	for _, attachment := range attachments {
		rows = append(rows, []string{
			outfmt.Sanitize(attachment.ID),
			outfmt.Sanitize(attachment.Name),
			outfmt.Sanitize(attachment.ContentType),
			fmt.Sprintf("%d", attachment.Size),
		})
	}
	return printer.Print(headers, rows, attachments, len(attachments), "")
}

// CalendarAttachmentsAddCmd adds a small file attachment to an event.
type CalendarAttachmentsAddCmd struct {
	EventID string `arg:"" help:"Event ID"`
	File    string `arg:"" help:"File to upload" type:"existingfile"`
}

func (c *CalendarAttachmentsAddCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	content, err := readLocalFile(c.File, graphapi.MaxCalendarAttachmentBytes-1)
	if err != nil {
		return fmt.Errorf("reading file (event attachments must be under 3 MB): %w", err)
	}

	name := filepath.Base(c.File)
	contentType := http.DetectContentType(content)
	if ctx.Flags.DryRun {
		fmt.Printf("Would upload %s to event %s%s\n", outfmt.Sanitize(name), outfmt.Sanitize(c.EventID), mailboxSuffix("in", target))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachment, err := client.UploadCalendarAttachment(ctx.Ctx, target, c.EventID, name, contentType, content)
	if err != nil {
		return err
	}
	if ctx.Flags.JSON {
		return ctx.Printer().PrintJSON(attachment, 1, "")
	}
	fmt.Printf("Uploaded: %s (ID: %s)\n", outfmt.Sanitize(attachment.Name), outfmt.Sanitize(attachment.ID))
	return nil
}

// CalendarAttachmentsDownloadCmd downloads an attachment from an event.
type CalendarAttachmentsDownloadCmd struct {
	EventID      string `arg:"" help:"Event ID"`
	AttachmentID string `arg:"" help:"Attachment ID"`
	Out          string `help:"Output directory for download" default:"." type:"path"`
}

func (c *CalendarAttachmentsDownloadCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	if ctx.Flags.DryRun {
		fmt.Printf("Would download attachment %s from event %s%s to %s\n", outfmt.Sanitize(c.AttachmentID),
			outfmt.Sanitize(c.EventID), mailboxSuffix("in", target), outfmt.Sanitize(c.Out))
		return nil
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachment, content, err := client.DownloadCalendarAttachment(ctx.Ctx, target, c.EventID, c.AttachmentID)
	if err != nil {
		return err
	}
	outDir, err := ctx.saveDir(c.Out)
	if err != nil {
		return err
	}
	saved, err := ctx.writeNew(outDir, sanitizeFilename(attachment.Name), content)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return printSaved(ctx, saved)
}

// CalendarAttachmentsDeleteCmd deletes an attachment from an event.
type CalendarAttachmentsDeleteCmd struct {
	EventID      string `arg:"" help:"Event ID"`
	AttachmentID string `arg:"" help:"Attachment ID"`
}

func (c *CalendarAttachmentsDeleteCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	if !ctx.Flags.Force {
		return fmt.Errorf("delete attachment %s: use --force to confirm deletion", outfmt.Sanitize(c.AttachmentID))
	}
	if ctx.Flags.DryRun {
		fmt.Printf("Would delete attachment %s from event %s%s\n", outfmt.Sanitize(c.AttachmentID), outfmt.Sanitize(c.EventID), mailboxSuffix("in", target))
		return nil
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	if err := client.DeleteCalendarAttachment(ctx.Ctx, target, c.EventID, c.AttachmentID); err != nil {
		return err
	}
	fmt.Println("Attachment deleted.")
	return nil
}
