package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
)

func prepareInlineAttachments(specs []string, body string) ([]graphapi.InlineAttachmentInput, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	attachments := make([]graphapi.InlineAttachmentInput, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		contentID, path, ok := strings.Cut(spec, "=")
		if !ok || contentID == "" || path == "" {
			return nil, fmt.Errorf("invalid --inline %q: expected CID=PATH", spec)
		}
		if err := graphapi.ValidateInlineContentID(contentID); err != nil {
			return nil, err
		}
		key := strings.ToLower(contentID)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate inline content ID %q", contentID)
		}
		seen[key] = struct{}{}

		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inline image %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("inline image %q is not a regular file", path)
		}
		if info.Size() >= graphapi.MaxInlineAttachmentBytes {
			return nil, fmt.Errorf("inline image %q is %d bytes; Graph simple attachments must be under 3 MB", path, info.Size())
		}

		content, err := readInlineImage(path)
		if err != nil {
			return nil, fmt.Errorf("reading inline image %q: %w", path, err)
		}
		if len(content) >= graphapi.MaxInlineAttachmentBytes {
			return nil, fmt.Errorf("inline image %q is %d bytes; Graph simple attachments must be under 3 MB", path, len(content))
		}
		contentType := http.DetectContentType(content)
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
			return nil, fmt.Errorf("inline image %q has non-image content type %q", path, contentType)
		}
		if !graphapi.HTMLReferencesInlineContentID(body, contentID) {
			return nil, fmt.Errorf("HTML body does not reference inline content ID %q as cid:%s", contentID, contentID)
		}

		attachments = append(attachments, graphapi.InlineAttachmentInput{
			ContentID:   contentID,
			Name:        filepath.Base(path),
			ContentType: contentType,
			Content:     content,
		})
	}
	return attachments, nil
}

func readInlineImage(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, graphapi.MaxInlineAttachmentBytes))
}
