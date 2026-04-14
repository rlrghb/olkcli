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

// validateOutDir ensures the output directory is not a symlink and exists.
func validateOutDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("checking output directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output directory %s is a symlink, refusing to write", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %s is not a directory", dir)
	}
	return nil
}

// safeWriteFile writes content to path, refusing to overwrite existing files.
// Appends a numeric suffix (e.g., "file(1).pdf") to avoid collisions.
func safeWriteFile(path string, content []byte) (string, error) {
	// Try the original path first with O_EXCL to prevent overwrite.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, writeErr := f.Write(content)
		closeErr := f.Close()
		if writeErr != nil {
			return path, writeErr
		}
		return path, closeErr
	}
	if !os.IsExist(err) {
		return "", err
	}

	// File exists — find an available name with a numeric suffix.
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		f, err = os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, writeErr := f.Write(content)
			closeErr := f.Close()
			if writeErr != nil {
				return candidate, writeErr
			}
			return candidate, closeErr
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not find available filename for %s after 1000 attempts", filepath.Base(path))
}

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
		// Size is validated in the API layer (graphapi/mail.go).

		outDir := c.Out
		if err := validateOutDir(outDir); err != nil {
			return err
		}

		filename := sanitizeFilename(att.Name)
		outPath := filepath.Join(outDir, filename)
		saved, err := safeWriteFile(outPath, att.Content)
		if err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		fmt.Printf("Saved: %s\n", saved)
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
		if err := validateOutDir(outDir); err != nil {
			return err
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
			saved, err := safeWriteFile(outPath, att.Content)
			if err != nil {
				return fmt.Errorf("writing file %q: %w", filename, err)
			}
			fmt.Printf("Saved: %s\n", saved)
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
