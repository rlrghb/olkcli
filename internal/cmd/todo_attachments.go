package cmd

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// TodoAttachCmd manages file attachments on tasks.
type TodoAttachCmd struct {
	List     TodoAttachListCmd     `cmd:"" help:"List task attachments"`
	Upload   TodoAttachUploadCmd   `cmd:"" help:"Upload a file attachment"`
	Download TodoAttachDownloadCmd `cmd:"" help:"Download an attachment"`
	Delete   TodoAttachDeleteCmd   `cmd:"" help:"Delete an attachment"`
}

// TodoAttachListCmd lists attachments on a task.
type TodoAttachListCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoAttachListCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	attachments, err := client.ListTodoAttachments(ctx.Ctx, listID, c.TaskID)
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

	headers := []string{"ID", "NAME", "TYPE", "SIZE"}
	var rows [][]string
	for _, a := range attachments {
		rows = append(rows, []string{
			outfmt.Sanitize(a.ID),
			outfmt.Sanitize(a.Name),
			outfmt.Sanitize(a.ContentType),
			fmt.Sprintf("%d", a.Size),
		})
	}

	return printer.Print(headers, rows, attachments, len(attachments), "")
}

// TodoAttachUploadCmd uploads a file attachment to a task.
type TodoAttachUploadCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	File   string `arg:"" help:"File to upload" type:"existingfile"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoAttachUploadCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	info, err := os.Stat(c.File)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if info.Size() > maxDownloadSize {
		return fmt.Errorf("file is %d bytes, exceeds 50MB upload limit", info.Size())
	}

	content, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	filename := filepath.Base(c.File)
	contentType := mime.TypeByExtension(filepath.Ext(c.File))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would upload %s to task %s\n", outfmt.Sanitize(filename), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	att, err := client.UploadTodoAttachment(ctx.Ctx, listID, c.TaskID, filename, contentType, content)
	if err != nil {
		return err
	}

	fmt.Printf("Uploaded: %s (ID: %s)\n", outfmt.Sanitize(att.Name), outfmt.Sanitize(att.ID))
	return nil
}

// TodoAttachDownloadCmd downloads an attachment from a task.
type TodoAttachDownloadCmd struct {
	TaskID       string `arg:"" help:"Task ID"`
	AttachmentID string `arg:"" help:"Attachment ID"`
	Out          string `help:"Output directory for download" default:"." type:"path"`
	List         string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoAttachDownloadCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	name, _, content, err := client.DownloadTodoAttachment(ctx.Ctx, listID, c.TaskID, c.AttachmentID)
	if err != nil {
		return err
	}

	if len(content) > maxDownloadSize {
		return fmt.Errorf("attachment is %d bytes, exceeds 50MB download limit", len(content))
	}

	if err := validateOutDir(c.Out); err != nil {
		return err
	}

	filename := sanitizeFilename(name)
	outPath := filepath.Join(c.Out, filename)
	saved, err := safeWriteFile(outPath, content)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Saved: %s\n", saved)
	return nil
}

// TodoAttachDeleteCmd deletes an attachment from a task.
type TodoAttachDeleteCmd struct {
	TaskID       string `arg:"" help:"Task ID"`
	AttachmentID string `arg:"" help:"Attachment ID"`
	List         string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoAttachDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete attachment %s: use --force to confirm deletion", outfmt.Sanitize(c.AttachmentID))
	}

	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete attachment %s from task %s\n", outfmt.Sanitize(c.AttachmentID), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.DeleteTodoAttachment(ctx.Ctx, listID, c.TaskID, c.AttachmentID)
	if err != nil {
		return err
	}

	fmt.Println("Attachment deleted.")
	return nil
}
