package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MailAttachmentsCmd struct {
	ID           string `arg:"" help:"Message ID"`
	Save         bool   `help:"Download all attachments" default:"false"`
	Out          string `help:"Output directory for downloads" default:"." type:"path"`
	AttachmentID string `help:"Download a specific attachment by ID" name:"attachment-id"`
}

// maxDownloadSize is the maximum size for a single attachment download (50 MB).
const maxDownloadSize = 50 << 20

// sanitizeFilename removes path separators and leading dots to prevent path traversal.
func sanitizeFilename(name string) string {
	// Strip any directory components
	name = filepath.Base(name)
	// Replace path separators that might remain
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	// Remove leading dots to prevent hidden files or traversal
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "attachment"
	}
	return name
}

func (c *MailAttachmentsCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Download a specific attachment by ID
	if c.AttachmentID != "" {
		att, err := client.DownloadAttachment(ctx.Ctx, c.ID, c.AttachmentID)
		if err != nil {
			return err
		}

		outDir := c.Out
		if err := os.MkdirAll(outDir, 0o750); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		filename := sanitizeFilename(att.Name)
		outPath := filepath.Join(outDir, filename)
		if err := os.WriteFile(outPath, att.Content, 0o600); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		fmt.Printf("Saved: %s\n", outPath)
		return nil
	}

	attachments, err := client.GetAttachments(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		fmt.Println("No attachments.")
		return nil
	}

	// Download all attachments if --save is set
	if c.Save {
		outDir := c.Out
		if err := os.MkdirAll(outDir, 0o750); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}

		for _, a := range attachments {
			if a.Size > maxDownloadSize {
				return fmt.Errorf("attachment %q is %d bytes, exceeds 50MB download limit", a.Name, a.Size)
			}
			att, err := client.DownloadAttachment(ctx.Ctx, c.ID, a.ID)
			if err != nil {
				return fmt.Errorf("downloading %q: %w", a.Name, err)
			}

			filename := sanitizeFilename(att.Name)
			outPath := filepath.Join(outDir, filename)
			if err := os.WriteFile(outPath, att.Content, 0o600); err != nil {
				return fmt.Errorf("writing file %q: %w", filename, err)
			}
			fmt.Printf("Saved: %s\n", outPath)
		}
		return nil
	}

	// Default: list attachments
	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(attachments, len(attachments), "")
	}

	headers := []string{"ID", "NAME", "TYPE", "SIZE"}
	var rows [][]string
	for _, a := range attachments {
		rows = append(rows, []string{
			a.ID,
			a.Name,
			a.ContentType,
			fmt.Sprintf("%d", a.Size),
		})
	}

	return printer.Print(headers, rows, attachments, len(attachments), "")
}
