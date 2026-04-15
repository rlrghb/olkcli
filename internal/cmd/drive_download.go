package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

const (
	// maxSmallDownload is the threshold for in-memory download (50 MB).
	maxSmallDownload = 50 << 20
	// maxDriveDownload is the hard limit for CLI downloads (2 GB).
	maxDriveDownload = 2 << 30
)

// DriveDownloadCmd downloads a file.
type DriveDownloadCmd struct {
	ID      string `arg:"" help:"Item ID"`
	Output  string `help:"Output directory" short:"o" default:"." type:"path"`
	DriveID string `help:"Drive ID (default: primary drive)" name:"drive-id" env:"OLK_DRIVE_ID"`
}

func (c *DriveDownloadCmd) Run(ctx *RunContext) error {
	if looksLikePath(c.ID) {
		return fmt.Errorf("argument looks like a path; use 'olk drive ls %s' to find item IDs", c.ID)
	}

	driveID, err := resolveDriveID(ctx, c.DriveID)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Get metadata first
	item, err := client.GetDriveItem(ctx.Ctx, driveID, c.ID)
	if err != nil {
		return err
	}

	if item.ItemType == driveItemTypeFolder {
		return fmt.Errorf("cannot download a folder; use a file item ID")
	}

	if item.Size > maxDriveDownload {
		return fmt.Errorf("file too large: %s (max %s)", formatBytes(item.Size), formatBytes(maxDriveDownload))
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would download %s (%s) to %s\n",
			outfmt.Sanitize(item.Name), formatBytes(item.Size), outfmt.Sanitize(c.Output))
		return nil
	}

	if err := validateOutDir(c.Output); err != nil {
		return err
	}

	filename := sanitizeFilename(item.Name)
	destPath := filepath.Join(c.Output, filename)

	if item.Size <= maxSmallDownload {
		// Small file: in-memory download
		content, err := client.DownloadDriveItem(ctx.Ctx, driveID, c.ID)
		if err != nil {
			return err
		}
		written, err := safeWriteFile(destPath, content)
		if err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		fmt.Printf("Downloaded %s (%s)\n", outfmt.Sanitize(filepath.Base(written)), formatBytes(item.Size))
		return nil
	}

	// Large file: streaming download via download URL
	if item.DownloadURL == "" {
		return fmt.Errorf("no download URL available for this item; try a smaller file or use the web interface")
	}
	if err := validateGraphURL(item.DownloadURL); err != nil {
		return fmt.Errorf("download URL rejected: %w", err)
	}

	f, finalPath, err := safeCreateFile(destPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	cleanupFile := func() {
		f.Close()
		os.Remove(finalPath)
	}

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, item.DownloadURL, http.NoBody)
	if err != nil {
		cleanupFile()
		return fmt.Errorf("creating download request failed")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cleanupFile()
		return fmt.Errorf("downloading file failed (check network connectivity)")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cleanupFile()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Limit read to declared file size + 1 byte to detect server sending more than expected.
	limitedReader := io.LimitReader(resp.Body, item.Size+1)
	written, err := io.Copy(f, limitedReader)
	if err != nil {
		cleanupFile()
		return fmt.Errorf("writing file: %w", err)
	}
	if written > item.Size {
		cleanupFile()
		return fmt.Errorf("server sent more data than expected (%s declared), aborting", formatBytes(item.Size))
	}

	f.Close()
	fmt.Printf("Downloaded %s (%s)\n", outfmt.Sanitize(filepath.Base(finalPath)), formatBytes(item.Size))
	return nil
}

// safeCreateFile opens a file for writing with O_EXCL to prevent overwriting.
// If the file exists, it tries numeric suffixes like file(1).pdf.
// Returns the open file handle and the final path used.
func safeCreateFile(path string) (*os.File, string, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		return f, path, nil
	}
	if !os.IsExist(err) {
		return nil, "", err
	}

	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s(%d)%s", base, i, ext)
		f, err = os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, candidate, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not find available filename for %s after 1000 attempts", filepath.Base(path))
}
