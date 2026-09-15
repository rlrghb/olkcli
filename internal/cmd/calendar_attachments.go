package cmd

import (
	"fmt"
	"net/http"
	"os"
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
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachments, err := client.ListCalendarAttachments(ctx.Ctx, c.EventID)
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
	info, err := os.Stat(c.File)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file %q is not a regular file", c.File)
	}
	if info.Size() >= graphapi.MaxCalendarAttachmentBytes {
		return fmt.Errorf("file is %d bytes; event attachments must be under 3 MB", info.Size())
	}
	content, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if len(content) >= graphapi.MaxCalendarAttachmentBytes {
		return fmt.Errorf("file is %d bytes; event attachments must be under 3 MB", len(content))
	}

	name := filepath.Base(c.File)
	contentType := http.DetectContentType(content)
	if ctx.Flags.DryRun {
		fmt.Printf("Would upload %s to event %s\n", outfmt.Sanitize(name), outfmt.Sanitize(c.EventID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachment, err := client.UploadCalendarAttachment(ctx.Ctx, c.EventID, name, contentType, content)
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
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	attachment, content, err := client.DownloadCalendarAttachment(ctx.Ctx, c.EventID, c.AttachmentID)
	if err != nil {
		return err
	}
	if err := validateOutDir(c.Out); err != nil {
		return err
	}
	outPath := filepath.Join(c.Out, sanitizeFilename(attachment.Name))
	saved, err := safeWriteFile(outPath, content)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	fmt.Printf("Saved: %s\n", saved)
	return nil
}

// CalendarAttachmentsDeleteCmd deletes an attachment from an event.
type CalendarAttachmentsDeleteCmd struct {
	EventID      string `arg:"" help:"Event ID"`
	AttachmentID string `arg:"" help:"Attachment ID"`
}

func (c *CalendarAttachmentsDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete attachment %s: use --force to confirm deletion", outfmt.Sanitize(c.AttachmentID))
	}
	if ctx.Flags.DryRun {
		fmt.Printf("Would delete attachment %s from event %s\n", outfmt.Sanitize(c.AttachmentID), outfmt.Sanitize(c.EventID))
		return nil
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	if err := client.DeleteCalendarAttachment(ctx.Ctx, c.EventID, c.AttachmentID); err != nil {
		return err
	}
	fmt.Println("Attachment deleted.")
	return nil
}
