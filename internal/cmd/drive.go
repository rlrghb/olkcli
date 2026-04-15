package cmd

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

// allowedDownloadHostSuffixes are trusted host suffixes for pre-authenticated URLs
// returned by Microsoft Graph. Requests to other hosts are rejected.
// allowedDownloadHostSuffixes are trusted host suffixes for pre-authenticated URLs
// returned by Microsoft Graph. Requests to other hosts are rejected.
//
// Sources:
//   - https://learn.microsoft.com/en-us/sharepoint/required-urls-and-ports
//   - https://learn.microsoft.com/en-us/microsoft-365/enterprise/urls-and-ip-address-ranges
var allowedDownloadHostSuffixes = []string{
	".sharepoint.com",               // SharePoint Online / OneDrive for Business
	".microsoftpersonalcontent.com", // Personal OneDrive content
	".microsoft.com",                // graph.microsoft.com and other Microsoft services
	".live.com",                     // *.storage.live.com, *.onedrive.live.com
	".live.net",                     // *.docs.live.net, *.apis.live.net
	".1drv.com",                     // *.files.1drv.com, *.up.1drv.com
	".1drv.ms",                      // OneDrive short URLs
	".svc.ms",                       // Microsoft service endpoints
}

// validateGraphURL checks that a pre-authenticated URL from Graph API is safe to use:
// must be HTTPS, must be a known Microsoft host, must not resolve to loopback/private IPs.
func validateGraphURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing non-HTTPS URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("URL has no host")
	}
	// Reject literal IP targets that are loopback/private
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
			return fmt.Errorf("refusing request to private/loopback address")
		}
	}
	// Check host against allowlist
	allowed := false
	for _, suffix := range allowedDownloadHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("refusing request to untrusted host %q", host)
	}
	// Resolve hostname and reject if any resolved IP is loopback/private (SSRF protection)
	if net.ParseIP(host) == nil {
		addrs, err := net.LookupHost(host)
		if err != nil {
			return fmt.Errorf("cannot resolve host %q", host)
		}
		for _, addr := range addrs {
			ip := net.ParseIP(addr)
			if ip == nil {
				continue
			}
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
				return fmt.Errorf("host %q resolves to private/loopback address", host)
			}
		}
	}
	return nil
}

// DriveCmd is the top-level command group for OneDrive file operations.
type DriveCmd struct {
	List     DriveListCmd     `cmd:"" help:"List all drives"`
	Info     DriveInfoCmd     `cmd:"" help:"Show drive details and quota"`
	Ls       DriveLsCmd       `cmd:"" help:"List folder contents"`
	Get      DriveGetCmd      `cmd:"" help:"Get item details"`
	Search   DriveSearchCmd   `cmd:"" help:"Search files"`
	Recent   DriveRecentCmd   `cmd:"" help:"Recently accessed files"`
	Shared   DriveSharedCmd   `cmd:"" help:"Files shared with me"`
	Download DriveDownloadCmd `cmd:"" help:"Download a file"`
	Upload   DriveUploadCmd   `cmd:"" help:"Upload a file"`
	Mkdir    DriveMkdirCmd    `cmd:"" help:"Create a folder"`
	Cp       DriveCpCmd       `cmd:"" help:"Copy a file or folder"`
	Mv       DriveMvCmd       `cmd:"" help:"Move or rename a file or folder"`
	Rm       DriveRmCmd       `cmd:"" help:"Delete a file or folder"`
	Share    DriveShareCmd    `cmd:"" help:"Create a sharing link"`
	Versions DriveVersionsCmd `cmd:"" help:"List file version history"`
}

// resolveDriveID returns the provided driveID, or auto-detects the default drive.
func resolveDriveID(ctx *RunContext, driveID string) (string, error) {
	if driveID != "" {
		return driveID, nil
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return "", err
	}
	drive, err := client.GetDrive(ctx.Ctx, "")
	if err != nil {
		return "", fmt.Errorf("auto-detecting drive: %w", err)
	}
	return drive.ID, nil
}

// formatBytes returns a human-readable byte size string.
func formatBytes(n int64) string {
	if n < 0 {
		return ""
	}
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	case n < gb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	}
}

// looksLikePath returns true if the string appears to be a path rather than an ID.
func looksLikePath(s string) bool {
	return s != "" && s[0] == '/'
}

// driveItemType constants for consistent comparison.
const (
	driveItemTypeFile   = "file"
	driveItemTypeFolder = "folder"
)

// printDriveItems prints a list of drive items in the standard table format.
func printDriveItems(ctx *RunContext, items []graphapi.DriveItem) error {
	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(items, len(items), "")
	}

	loc, _ := ctx.Timezone()
	headers := []string{"NAME", "TYPE", "SIZE", "MODIFIED", "ID"}
	var rows [][]string
	for i := range items {
		item := &items[i]
		size := ""
		if item.ItemType == driveItemTypeFile {
			size = formatBytes(item.Size)
		}
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(item.Name), 50),
			outfmt.Sanitize(item.ItemType),
			size,
			outfmt.Truncate(outfmt.Sanitize(outfmt.ConvertTime(item.ModifiedAt, loc)), 16),
			outfmt.Truncate(outfmt.Sanitize(item.ID), 15),
		})
	}
	return printer.Print(headers, rows, items, len(items), "")
}
