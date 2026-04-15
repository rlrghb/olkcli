package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

const (
	// maxSimpleUpload is the threshold for simple PUT upload (4 MB).
	maxSimpleUpload = 4 << 20
	// uploadChunkSize is the chunk size for resumable uploads (10 MB, multiple of 320 KB).
	uploadChunkSize = 10 * 1024 * 1024
)

// DriveUploadCmd uploads a file.
type DriveUploadCmd struct {
	LocalPath  string `arg:"" help:"Local file path" type:"existingfile"`
	RemotePath string `arg:"" help:"Remote path (e.g. /Documents/report.pdf)"`
	DriveID    string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
	Replace    bool   `help:"Replace existing file" default:"false"`
}

func (c *DriveUploadCmd) Run(ctx *RunContext) error {
	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	info, err := os.Stat(c.LocalPath)
	if err != nil {
		return fmt.Errorf("reading local file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("cannot upload a directory")
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would upload %s (%s) to %s\n",
			outfmt.Sanitize(filepath.Base(c.LocalPath)),
			formatBytes(info.Size()),
			outfmt.Sanitize(c.RemotePath))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if info.Size() < maxSimpleUpload {
		// Simple upload
		content, err := os.ReadFile(c.LocalPath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		item, err := client.UploadSmallFile(ctx.Ctx, driveID, c.RemotePath, content, c.Replace)
		if err != nil {
			return err
		}
		fmt.Printf("Uploaded %s (%s) ID: %s\n",
			outfmt.Sanitize(item.Name), formatBytes(item.Size), outfmt.Sanitize(item.ID))
		return nil
	}

	// Resumable upload for large files
	uploadURL, err := client.CreateUploadSession(ctx.Ctx, driveID, c.RemotePath, c.Replace)
	if err != nil {
		return err
	}
	if err := validateGraphURL(uploadURL); err != nil {
		return fmt.Errorf("upload URL rejected: %w", err)
	}

	f, err := os.Open(c.LocalPath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	totalSize := info.Size()
	buf := make([]byte, uploadChunkSize)
	var offset int64

	for offset < totalSize {
		n, err := f.Read(buf)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("reading file chunk: %w", err)
		}
		if n == 0 {
			break
		}

		end := offset + int64(n) - 1
		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, end, totalSize)

		req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodPut, uploadURL, bytes.NewReader(buf[:n]))
		if err != nil {
			return fmt.Errorf("creating upload request failed")
		}
		req.Header.Set("Content-Range", contentRange)
		req.ContentLength = int64(n)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("uploading chunk: %w", err)
		}

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			return fmt.Errorf("upload chunk failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		// Drain and close body for connection reuse
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain body for connection reuse
		resp.Body.Close()

		offset += int64(n)
		pct := int(float64(offset) / float64(totalSize) * 100)
		fmt.Fprintf(os.Stderr, "\rUploading... %d%%", pct)
	}
	fmt.Fprintln(os.Stderr)

	fmt.Printf("Uploaded %s (%s)\n",
		outfmt.Sanitize(filepath.Base(c.LocalPath)), formatBytes(totalSize))
	return nil
}
