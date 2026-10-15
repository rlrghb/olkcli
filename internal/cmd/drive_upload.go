package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
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
		content, err := readLocalFile(c.LocalPath, maxSimpleUpload-1)
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
	// re-validates redirects + denies internal IPs at dial time
	if err := uploadChunks(ctx.Ctx, hardenedDownloadClient(), uploadURL, f, totalSize); err != nil {
		return err
	}

	fmt.Printf("Uploaded %s (%s)\n",
		outfmt.Sanitize(filepath.Base(c.LocalPath)), formatBytes(totalSize))
	return nil
}

// uploadChunks sends totalSize bytes of f to a resumable upload session, and
// succeeds only once the server confirms the final range.
func uploadChunks(ctx context.Context, httpClient *http.Client, uploadURL string, f io.Reader, totalSize int64) error {
	buf := make([]byte, uploadChunkSize)
	var offset int64

	finished := false
	for offset < totalSize {
		n, err := io.ReadFull(f, buf[:min(int64(len(buf)), totalSize-offset)])
		if err != nil {
			// A file that shrank since it was measured cannot complete the
			// upload session it declared.
			return fmt.Errorf("reading file chunk at byte %d of %d: %w", offset, totalSize, err)
		}

		end := offset + int64(n) - 1
		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, end, totalSize)

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(buf[:n]))
		if err != nil {
			return fmt.Errorf("creating upload request failed")
		}
		req.Header.Set("Content-Range", contentRange)
		req.ContentLength = int64(n)

		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("uploading chunk: %w", transportFailure(err))
		}

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			return fmt.Errorf("upload chunk failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		// Graph answers 202 until the last range, then 200 or 201 with the item.
		finished = resp.StatusCode != http.StatusAccepted
		// Drain and close body for connection reuse
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain body for connection reuse
		resp.Body.Close()

		offset += int64(n)
		pct := int(float64(offset) / float64(totalSize) * 100)
		fmt.Fprintf(os.Stderr, "\rUploading... %d%%", pct)
	}
	fmt.Fprintln(os.Stderr)
	if !finished {
		return fmt.Errorf("upload incomplete: the server never confirmed the final range")
	}

	return nil
}

// transportFailure describes a failed request without its error text. The
// upload URL is pre-authenticated, and Go's errors quote it: *url.Error does,
// and so does a malformed redirect's Location, which may carry a token too.
// It names the kind of failure from the error's types alone.
func transportFailure(err error) error {
	var (
		ne      net.Error
		dnsErr  *net.DNSError
		certErr *tls.CertificateVerificationError
		unknown x509.UnknownAuthorityError
		invalid x509.CertificateInvalidError
		host    x509.HostnameError
	)
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.As(err, &certErr), errors.As(err, &unknown), errors.As(err, &invalid), errors.As(err, &host):
		return errors.New("TLS certificate verification failed")
	case errors.As(err, &dnsErr):
		return errors.New("DNS lookup failed")
	case errors.As(err, &ne) && ne.Timeout():
		return errors.New("timed out")
	}
	return errors.New("connection failed")
}
