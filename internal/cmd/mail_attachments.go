package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
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

// sanitizeFilename removes path separators, control characters, and leading dots
// to prevent path traversal and filename-based attacks.
func sanitizeFilename(name string) string {
	// Strip any directory components
	name = filepath.Base(name)
	// Replace path separators that might remain
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	// Remove null bytes and control characters (U+0000–U+001F, U+007F)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, name)
	// Remove leading dots to prevent hidden files or traversal
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "attachment"
	}
	return name
}

// attachmentSaveResult reports one attachment of a --save run. The name, the
// path built from it and any error that quotes it all come from the sender.
type attachmentSaveResult struct {
	ID    string `json:"id"`
	Name  string `json:"name" untrusted:"true"`
	Path  string `json:"path,omitempty" untrusted:"true"`
	Error string `json:"error,omitempty" untrusted:"true"`
}

// saveAll downloads every attachment into --out. One attachment that cannot be
// saved does not stop the rest, and the command fails at the end so a script
// still sees it. With --json or --wrap-untrusted the outcome of each
// attachment is printed as one structured list, so a sender-chosen name never
// reaches an agent unmarked; otherwise each failure goes to stderr as it
// happens.
func (c *MailAttachmentsCmd) saveAll(
	ctx *RunContext, client *graphapi.Client, target string, attachments []graphapi.Attachment,
) error {
	if err := validateOutDir(c.Out); err != nil {
		return err
	}
	structured := ctx.Flags.JSON || ctx.Flags.WrapUntrusted
	results := make([]attachmentSaveResult, 0, len(attachments))
	failed := 0
	for i := range attachments {
		a := &attachments[i]
		result := attachmentSaveResult{ID: a.ID, Name: a.Name}
		saved, err := saveAttachment(ctx, client, target, c.ID, a, c.Out)
		switch {
		case err != nil:
			failed++
			result.Error = err.Error()
			if !structured {
				fmt.Fprintf(os.Stderr, "Failed: %s: %s\n", outfmt.Sanitize(a.Name), outfmt.Sanitize(err.Error()))
			}
		case !structured:
			fmt.Printf("Saved: %s\n", saved)
		}
		result.Path = saved
		results = append(results, result)
	}
	if structured {
		if err := ctx.Printer().PrintJSON(results, len(results), ""); err != nil {
			return err
		}
	}
	if failed > 0 {
		err := fmt.Errorf("%d of %d attachments could not be saved", failed, len(attachments))
		if structured {
			return &reportedInOutputError{err: err}
		}
		return err
	}
	return nil
}

func saveAttachment(
	ctx *RunContext, client *graphapi.Client, target, messageID string, a *graphapi.Attachment, outDir string,
) (string, error) {
	if a.Size > maxDownloadSize {
		return "", fmt.Errorf("%d bytes exceeds the 50MB download limit", a.Size)
	}
	att, err := client.DownloadAttachment(ctx.Ctx, target, messageID, a.ID)
	if err != nil {
		return "", err
	}
	filename := sanitizeFilename(att.Name)
	saved, err := safeWriteFile(filepath.Join(outDir, filename), att.Content)
	if err != nil {
		return "", fmt.Errorf("writing file %q: %w", filename, err)
	}
	return saved, nil
}

func (c *MailAttachmentsCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	if ctx.Flags.DryRun && (c.Save || c.AttachmentID != "") {
		if c.AttachmentID != "" {
			fmt.Printf("Would download attachment %s", outfmt.Sanitize(c.AttachmentID))
		} else {
			fmt.Print("Would save attachments")
		}
		fmt.Printf(" from message %s in %s to %s\n", outfmt.Sanitize(c.ID), describeMailbox(target), outfmt.Sanitize(c.Out))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Download a specific attachment by ID
	if c.AttachmentID != "" {
		att, err := client.DownloadAttachment(ctx.Ctx, target, c.ID, c.AttachmentID)
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

	attachments, err := client.GetAttachments(ctx.Ctx, target, c.ID)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		fmt.Println("No attachments.")
		return nil
	}

	// Download all attachments if --save is set
	if c.Save {
		return c.saveAll(ctx, client, target, attachments)
	}

	// Default: list attachments
	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(attachments, len(attachments), "")
	}

	headers := []string{"ID", "NAME", "TYPE", "SIZE"}
	rows := make([][]string, 0, len(attachments))
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
