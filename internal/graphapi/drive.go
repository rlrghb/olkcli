package graphapi

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/drives"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// safeDriveSearchQuery matches alphanumeric, Unicode letters, spaces, @, dots, hyphens, underscores.
// SECURITY: this whitelist prevents KQL injection in drive search queries.
var safeDriveSearchQuery = regexp.MustCompile(`^[\p{L}\p{N} @._-]+$`)

// DriveInfo is a simplified drive for output.
type DriveInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	DriveType      string `json:"driveType"`
	QuotaTotal     int64  `json:"quotaTotal"`
	QuotaUsed      int64  `json:"quotaUsed"`
	QuotaRemaining int64  `json:"quotaRemaining"`
	QuotaState     string `json:"quotaState"`
	OwnerName      string `json:"ownerName,omitempty"`
	OwnerEmail     string `json:"ownerEmail,omitempty"`
	WebURL         string `json:"webUrl"`
}

// DriveItem is a simplified drive item for output.
type DriveItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ItemType    string `json:"itemType"`
	MimeType    string `json:"mimeType,omitempty"`
	CreatedAt   string `json:"createdDateTime"`
	ModifiedAt  string `json:"lastModifiedDateTime"`
	WebURL      string `json:"webUrl"`
	DownloadURL string `json:"-"` // pre-authenticated URL with embedded token; excluded from JSON for security
	ParentPath  string `json:"parentPath,omitempty"`
	ChildCount  int32  `json:"childCount,omitempty"`
	CreatedBy   string `json:"createdBy,omitempty"`
	ModifiedBy  string `json:"modifiedBy,omitempty"`
}

// DriveItemVersion is a simplified version entry for output.
type DriveItemVersion struct {
	ID         string `json:"id"`
	ModifiedAt string `json:"lastModifiedDateTime"`
	Size       int64  `json:"size"`
	ModifiedBy string `json:"lastModifiedBy,omitempty"`
}

// ShareLink is a simplified sharing link for output.
type ShareLink struct {
	ID       string `json:"id"`
	LinkType string `json:"type"`
	URL      string `json:"webUrl"`
	Scope    string `json:"scope"`
}

func convertDrive(d models.Driveable) DriveInfo {
	info := DriveInfo{
		WebURL: derefStr(d.GetWebUrl()),
	}
	if d.GetId() != nil {
		info.ID = *d.GetId()
	}
	if d.GetName() != nil {
		info.Name = *d.GetName()
	}
	if d.GetDriveType() != nil {
		info.DriveType = *d.GetDriveType()
	}
	if q := d.GetQuota(); q != nil {
		if q.GetTotal() != nil {
			info.QuotaTotal = *q.GetTotal()
		}
		if q.GetUsed() != nil {
			info.QuotaUsed = *q.GetUsed()
		}
		if q.GetRemaining() != nil {
			info.QuotaRemaining = *q.GetRemaining()
		}
		if q.GetState() != nil {
			info.QuotaState = *q.GetState()
		}
	}
	if o := d.GetOwner(); o != nil {
		if u := o.GetUser(); u != nil {
			info.OwnerName = derefStr(u.GetDisplayName())
			// User identity doesn't expose email directly; use additionalData.
			// Kiota may deserialize as string or *string depending on version.
			if email, ok := u.GetAdditionalData()["email"]; ok {
				switch v := email.(type) {
				case string:
					info.OwnerEmail = v
				case *string:
					if v != nil {
						info.OwnerEmail = *v
					}
				}
			}
		}
	}
	return info
}

func convertDriveItem(d models.DriveItemable) DriveItem {
	item := DriveItem{
		WebURL: derefStr(d.GetWebUrl()),
	}
	if d.GetId() != nil {
		item.ID = *d.GetId()
	}
	if d.GetName() != nil {
		item.Name = *d.GetName()
	}
	if d.GetSize() != nil {
		item.Size = *d.GetSize()
	}
	if d.GetFolder() != nil {
		item.ItemType = "folder"
		if d.GetFolder().GetChildCount() != nil {
			item.ChildCount = *d.GetFolder().GetChildCount()
		}
	} else if d.GetFile() != nil {
		item.ItemType = "file"
		if d.GetFile().GetMimeType() != nil {
			item.MimeType = *d.GetFile().GetMimeType()
		}
	}
	if ct := d.GetCreatedDateTime(); ct != nil {
		item.CreatedAt = ct.UTC().Format(time.RFC3339)
	}
	if mt := d.GetLastModifiedDateTime(); mt != nil {
		item.ModifiedAt = mt.UTC().Format(time.RFC3339)
	}
	if ref := d.GetParentReference(); ref != nil {
		item.ParentPath = derefStr(ref.GetPath())
	}
	if cb := d.GetCreatedBy(); cb != nil {
		if u := cb.GetUser(); u != nil {
			item.CreatedBy = derefStr(u.GetDisplayName())
		}
	}
	if mb := d.GetLastModifiedBy(); mb != nil {
		if u := mb.GetUser(); u != nil {
			item.ModifiedBy = derefStr(u.GetDisplayName())
		}
	}
	// Extract download URL from additional data.
	// Kiota may deserialize this as string or *string depending on version.
	if dlURL, ok := d.GetAdditionalData()["@microsoft.graph.downloadUrl"]; ok {
		switch v := dlURL.(type) {
		case string:
			item.DownloadURL = v
		case *string:
			if v != nil {
				item.DownloadURL = *v
			}
		}
	}
	return item
}

func convertDriveItemVersion(v models.DriveItemVersionable) DriveItemVersion {
	ver := DriveItemVersion{}
	if v.GetId() != nil {
		ver.ID = *v.GetId()
	}
	if mt := v.GetLastModifiedDateTime(); mt != nil {
		ver.ModifiedAt = mt.UTC().Format(time.RFC3339)
	}
	if v.GetSize() != nil {
		ver.Size = *v.GetSize()
	}
	if mb := v.GetLastModifiedBy(); mb != nil {
		if u := mb.GetUser(); u != nil {
			ver.ModifiedBy = derefStr(u.GetDisplayName())
		}
	}
	return ver
}

// validateDrivePath validates a OneDrive path for safety.
func validateDrivePath(path string) error {
	if strings.ContainsRune(path, '\x00') {
		return fmt.Errorf("path contains null byte")
	}
	if len(path) > 400 {
		return fmt.Errorf("path too long: %d characters (max 400)", len(path))
	}
	clean := strings.Trim(path, "/")
	if clean == "" {
		return nil
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" {
			return fmt.Errorf("path contains empty segment")
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("path contains unsafe segment %q", seg)
		}
	}
	return nil
}

// encodePathSegments URL-encodes each path segment individually.
func encodePathSegments(path string) string {
	clean := strings.Trim(path, "/")
	if clean == "" {
		return ""
	}
	parts := strings.Split(clean, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// ListDrives returns all drives for the current user.
func (c *Client) ListDrives(ctx context.Context) ([]DriveInfo, error) {
	resp, err := c.inner.Me().Drives().Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("listing drives", err)
	}
	var result []DriveInfo
	for _, d := range resp.GetValue() {
		result = append(result, convertDrive(d))
	}
	return result, nil
}

// GetDrive returns details for a specific drive, or the default drive if driveID is empty.
func (c *Client) GetDrive(ctx context.Context, driveID string) (*DriveInfo, error) {
	var d models.Driveable
	var err error
	if driveID == "" {
		d, err = c.inner.Me().Drive().Get(ctx, nil)
	} else {
		if e := validateID(driveID, "drive ID"); e != nil {
			return nil, e
		}
		d, err = c.inner.Drives().ByDriveId(driveID).Get(ctx, nil)
	}
	if err != nil {
		return nil, scopeUpgradeError("getting drive", err)
	}
	info := convertDrive(d)
	return &info, nil
}

// ListDriveChildren returns items in a folder by item ID.
func (c *Client) ListDriveChildren(ctx context.Context, driveID, itemID string, top int32) ([]DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if itemID == "" {
		itemID = "root"
	}
	top = clampTop(top)
	topVal := top
	orderBy := "name asc"
	config := &drives.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{
		QueryParameters: &drives.ItemItemsItemChildrenRequestBuilderGetQueryParameters{
			Top:     &topVal,
			Orderby: []string{orderBy},
		},
	}
	resp, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Children().Get(ctx, config)
	if err != nil {
		return nil, scopeUpgradeError("listing folder contents", err)
	}
	var result []DriveItem
	for _, d := range resp.GetValue() {
		result = append(result, convertDriveItem(d))
	}
	return result, nil
}

// ListDriveChildrenByPath returns items in a folder by path.
func (c *Client) ListDriveChildrenByPath(ctx context.Context, driveID, path string, top int32) ([]DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateDrivePath(path); err != nil {
		return nil, err
	}
	clean := strings.Trim(path, "/")
	if clean == "" {
		return c.ListDriveChildren(ctx, driveID, "root", top)
	}
	top = clampTop(top)
	topVal := top
	orderBy := "name asc"
	encoded := encodePathSegments(clean)
	rawURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:/children",
		url.PathEscape(driveID), encoded)
	builder := drives.NewItemItemsItemChildrenRequestBuilder(rawURL, c.inner.GetAdapter())
	config := &drives.ItemItemsItemChildrenRequestBuilderGetRequestConfiguration{
		QueryParameters: &drives.ItemItemsItemChildrenRequestBuilderGetQueryParameters{
			Top:     &topVal,
			Orderby: []string{orderBy},
		},
	}
	resp, err := builder.Get(ctx, config)
	if err != nil {
		return nil, scopeUpgradeError("listing folder contents", err)
	}
	var result []DriveItem
	for _, d := range resp.GetValue() {
		result = append(result, convertDriveItem(d))
	}
	return result, nil
}

// GetDriveItem returns metadata for a specific item.
func (c *Client) GetDriveItem(ctx context.Context, driveID, itemID string) (*DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("getting item", err)
	}
	item := convertDriveItem(resp)
	return &item, nil
}

// SearchDrive searches for items matching a query.
func (c *Client) SearchDrive(ctx context.Context, driveID, query string, top int32) ([]DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}
	if len(query) > 1024 {
		return nil, fmt.Errorf("search query too long: %d characters (max 1024)", len(query))
	}
	if !safeDriveSearchQuery.MatchString(query) {
		return nil, fmt.Errorf("search query contains invalid characters")
	}
	top = clampTop(top)
	resp, err := c.inner.Drives().ByDriveId(driveID).SearchWithQ(&query).Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("searching drive", err)
	}
	var result []DriveItem
	for _, d := range resp.GetValue() {
		result = append(result, convertDriveItem(d))
	}
	// Limit results to top
	if int32(len(result)) > top {
		result = result[:top]
	}
	return result, nil
}

// RecentDriveItems returns recently accessed items.
func (c *Client) RecentDriveItems(ctx context.Context, driveID string) ([]DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Drives().ByDriveId(driveID).Recent().Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("getting recent items", err)
	}
	var result []DriveItem
	for _, d := range resp.GetValue() {
		result = append(result, convertDriveItem(d))
	}
	return result, nil
}

// SharedWithMeItems returns items shared with the current user.
func (c *Client) SharedWithMeItems(ctx context.Context, driveID string) ([]DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Drives().ByDriveId(driveID).SharedWithMe().Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("getting shared items", err)
	}
	var result []DriveItem
	for _, d := range resp.GetValue() {
		result = append(result, convertDriveItem(d))
	}
	return result, nil
}

// DownloadDriveItem downloads file content by item ID.
// For large files, use GetDriveItem to get the DownloadURL and stream directly.
func (c *Client) DownloadDriveItem(ctx context.Context, driveID, itemID string) ([]byte, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return nil, err
	}
	content, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Content().Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("downloading item", err)
	}
	return content, nil
}

// UploadSmallFile uploads a file under 4MB via simple PUT.
func (c *Client) UploadSmallFile(ctx context.Context, driveID, remotePath string, content []byte, replace bool) (*DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateDrivePath(remotePath); err != nil {
		return nil, err
	}
	encoded := encodePathSegments(strings.Trim(remotePath, "/"))
	conflict := "fail"
	if replace {
		conflict = "replace"
	}
	rawURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:/content?@microsoft.graph.conflictBehavior=%s",
		url.PathEscape(driveID), encoded, conflict)
	builder := drives.NewItemItemsItemContentRequestBuilder(rawURL, c.inner.GetAdapter())
	resp, err := builder.Put(ctx, content, nil)
	if err != nil {
		return nil, scopeUpgradeError("uploading file", err)
	}
	item := convertDriveItem(resp)
	return &item, nil
}

// CreateUploadSession creates a resumable upload session for large files (>= 4MB).
func (c *Client) CreateUploadSession(ctx context.Context, driveID, remotePath string, replace bool) (string, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return "", err
	}
	if err := validateDrivePath(remotePath); err != nil {
		return "", err
	}
	encoded := encodePathSegments(strings.Trim(remotePath, "/"))
	rawURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:/createUploadSession",
		url.PathEscape(driveID), encoded)

	// Build request body with conflict behavior
	body := drives.NewItemItemsItemCreateUploadSessionPostRequestBody()
	props := models.NewDriveItemUploadableProperties()
	conflict := "fail"
	if replace {
		conflict = "replace"
	}
	props.SetAdditionalData(map[string]interface{}{
		"@microsoft.graph.conflictBehavior": conflict,
	})
	body.SetItem(props)

	builder := drives.NewItemItemsItemCreateUploadSessionRequestBuilder(rawURL, c.inner.GetAdapter())
	session, err := builder.Post(ctx, body, nil)
	if err != nil {
		return "", scopeUpgradeError("creating upload session", err)
	}
	if session.GetUploadUrl() == nil {
		return "", fmt.Errorf("upload session created but no upload URL returned")
	}
	return *session.GetUploadUrl(), nil
}

// CreateFolder creates a new folder under the given parent item ID.
func (c *Client) CreateFolder(ctx context.Context, driveID, parentItemID, folderName string) (*DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if parentItemID == "" {
		parentItemID = "root"
	}
	if folderName == "" {
		return nil, fmt.Errorf("folder name cannot be empty")
	}
	newItem := models.NewDriveItem()
	newItem.SetName(&folderName)
	folder := models.NewFolder()
	newItem.SetFolder(folder)
	// Set conflict behavior to fail
	newItem.SetAdditionalData(map[string]interface{}{
		"@microsoft.graph.conflictBehavior": "fail",
	})

	resp, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(parentItemID).Children().Post(ctx, newItem, nil)
	if err != nil {
		return nil, scopeUpgradeError("creating folder", err)
	}
	item := convertDriveItem(resp)
	return &item, nil
}

// CopyDriveItem initiates an async copy of an item to a new parent.
func (c *Client) CopyDriveItem(ctx context.Context, driveID, itemID, destParentID, newName string) error {
	if err := validateID(driveID, "drive ID"); err != nil {
		return err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return err
	}
	if err := validateID(destParentID, "destination parent ID"); err != nil {
		return err
	}

	body := drives.NewItemItemsItemCopyPostRequestBody()
	parentRef := models.NewItemReference()
	parentRef.SetDriveId(&driveID)
	parentRef.SetId(&destParentID)
	body.SetParentReference(parentRef)
	if newName != "" {
		body.SetName(&newName)
	}

	_, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Copy().Post(ctx, body, nil)
	if err != nil {
		return scopeUpgradeError("copying item", err)
	}
	return nil
}

// MoveDriveItem moves or renames an item.
func (c *Client) MoveDriveItem(ctx context.Context, driveID, itemID, destParentID, newName string) (*DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return nil, err
	}

	patch := models.NewDriveItem()
	if destParentID != "" {
		if err := validateID(destParentID, "destination parent ID"); err != nil {
			return nil, err
		}
		parentRef := models.NewItemReference()
		parentRef.SetId(&destParentID)
		patch.SetParentReference(parentRef)
	}
	if newName != "" {
		patch.SetName(&newName)
	}

	resp, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Patch(ctx, patch, nil)
	if err != nil {
		return nil, scopeUpgradeError("moving item", err)
	}
	item := convertDriveItem(resp)
	return &item, nil
}

// DeleteDriveItem deletes an item.
func (c *Client) DeleteDriveItem(ctx context.Context, driveID, itemID string) error {
	if err := validateID(driveID, "drive ID"); err != nil {
		return err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return err
	}
	err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Delete(ctx, nil)
	if err != nil {
		return scopeUpgradeError("deleting item", err)
	}
	return nil
}

// CreateShareLink creates a sharing link for an item.
func (c *Client) CreateShareLink(ctx context.Context, driveID, itemID, linkType, scope string) (*ShareLink, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return nil, err
	}

	body := drives.NewItemItemsItemCreateLinkPostRequestBody()
	body.SetTypeEscaped(&linkType)
	body.SetScope(&scope)

	perm, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).CreateLink().Post(ctx, body, nil)
	if err != nil {
		return nil, scopeUpgradeError("creating share link", err)
	}

	link := &ShareLink{}
	if perm.GetId() != nil {
		link.ID = *perm.GetId()
	}
	if l := perm.GetLink(); l != nil {
		if l.GetTypeEscaped() != nil {
			link.LinkType = *l.GetTypeEscaped()
		}
		if l.GetWebUrl() != nil {
			link.URL = *l.GetWebUrl()
		}
		if l.GetScope() != nil {
			link.Scope = *l.GetScope()
		}
	}
	return link, nil
}

// ListDriveItemVersions returns version history for an item.
func (c *Client) ListDriveItemVersions(ctx context.Context, driveID, itemID string) ([]DriveItemVersion, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "item ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Drives().ByDriveId(driveID).Items().ByDriveItemId(itemID).Versions().Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("listing versions", err)
	}
	var result []DriveItemVersion
	for _, v := range resp.GetValue() {
		result = append(result, convertDriveItemVersion(v))
	}
	return result, nil
}

// ResolveItemByPath resolves a path to a DriveItem using the Graph API path syntax.
func (c *Client) ResolveItemByPath(ctx context.Context, driveID, path string) (*DriveItem, error) {
	if err := validateID(driveID, "drive ID"); err != nil {
		return nil, err
	}
	if err := validateDrivePath(path); err != nil {
		return nil, err
	}
	clean := strings.Trim(path, "/")
	if clean == "" {
		// Root item
		resp, err := c.inner.Drives().ByDriveId(driveID).Root().Get(ctx, nil)
		if err != nil {
			return nil, scopeUpgradeError("resolving path", err)
		}
		item := convertDriveItem(resp)
		return &item, nil
	}
	encoded := encodePathSegments(clean)
	rawURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:",
		url.PathEscape(driveID), encoded)
	builder := drives.NewItemItemsDriveItemItemRequestBuilder(rawURL, c.inner.GetAdapter())
	resp, err := builder.Get(ctx, nil)
	if err != nil {
		return nil, scopeUpgradeError("resolving path", err)
	}
	item := convertDriveItem(resp)
	return &item, nil
}
