package cmd

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

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
	// Reject literal IP targets in a denied range.
	if ip := net.ParseIP(host); ip != nil && isDeniedIP(ip) {
		return fmt.Errorf("refusing request to private/loopback address")
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
			if ip := net.ParseIP(addr); isDeniedIP(ip) {
				return fmt.Errorf("host %q resolves to private/loopback address", host)
			}
		}
	}
	return nil
}

// isDeniedIP reports whether ip must never be the target of a request built from
// a Graph-supplied or model-supplied URL: loopback, private (RFC 1918), CGNAT
// (100.64.0.0/10), link-local (incl. the cloud-metadata 169.254.169.254), and
// the unspecified address. Used both at validation time and — to defeat DNS
// rebinding — at connect time via the dialer Control hook below.
func isDeniedIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true // 100.64.0.0/10 (carrier-grade NAT)
	}
	return false
}

// hardenedDownloadClient returns an http.Client for fetching/uploading to a
// Graph pre-authenticated URL that (1) re-validates every redirect hop with
// validateGraphURL — http.DefaultClient would follow a 302 to an internal host
// unchecked — and (2) re-checks the actually-dialed IP against isDeniedIP at
// connect time, closing the DNS-resolve-then-dial (rebinding) TOCTOU that a
// validation-time lookup alone cannot.
func hardenedDownloadClient() *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if isDeniedIP(net.ParseIP(host)) {
			return fmt.Errorf("refusing connection to disallowed address %s", host)
		}
		return nil
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 15 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return validateGraphURL(req.URL.String())
		},
	}
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
	rows := make([][]string, 0, len(items))
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
