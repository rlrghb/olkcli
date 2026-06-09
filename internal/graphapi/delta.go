package graphapi

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	abs "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// maxPageSizeHeaders expresses a delta page-size preference. The /delta
// endpoints reject the $top query option (page size can't be guaranteed across
// change tracking), so the size is requested via the Prefer header instead.
func maxPageSizeHeaders(top int32) *abs.RequestHeaders {
	if top <= 0 {
		return nil
	}
	h := abs.NewRequestHeaders()
	h.Add("Prefer", fmt.Sprintf("odata.maxpagesize=%d", top))
	return h
}

// Delta sync wraps Microsoft Graph's /delta endpoints for mail, calendar, and
// contacts. Each call returns the changes since a prior opaque token plus a new
// token to use next time — stateless, agent-driven, no server state (mirroring
// outlook-mcp's cursor model). One call returns one page: when Complete is
// false a nextLink token is returned (more changes available right now); when
// Complete is true the token is a deltaLink (caught up — store it for the next
// sync). An item with Removed=true is a deletion (only its ID is populated).

// DeltaPage is the continuation cursor returned by a delta call.
type DeltaPage struct {
	Token    string `json:"deltaToken,omitempty"` // opaque cursor; pass back to fetch the next page/changes
	Complete bool   `json:"complete"`             // true when caught up (token is a deltaLink); false = more pages now
}

// MailDelta is a changed message; Removed marks a deletion.
type MailDelta struct {
	MailMessage
	Removed bool `json:"removed,omitempty"`
}

// CalendarDelta is a changed event; Removed marks a deletion.
type CalendarDelta struct {
	CalendarEvent
	Removed bool `json:"removed,omitempty"`
}

// ContactDelta is a changed contact; Removed marks a deletion.
type ContactDelta struct {
	Contact
	Removed bool `json:"removed,omitempty"`
}

// deltaLinker is satisfied by every delta GET response (via the SDK's
// BaseDeltaFunctionResponse) and exposes the continuation links.
type deltaLinker interface {
	GetOdataDeltaLink() *string
	GetOdataNextLink() *string
}

// deltaPageFrom picks the continuation token: a deltaLink means caught up, a
// nextLink means more pages remain now.
func deltaPageFrom(r deltaLinker) DeltaPage {
	if dl := r.GetOdataDeltaLink(); dl != nil && *dl != "" {
		return DeltaPage{Token: *dl, Complete: true}
	}
	if nl := r.GetOdataNextLink(); nl != nil && *nl != "" {
		return DeltaPage{Token: *nl, Complete: false}
	}
	return DeltaPage{Complete: true}
}

// graphAPIHosts are the Microsoft Graph endpoints a delta continuation token may
// target. A token is a full Graph URL replayed with the authenticated adapter,
// so it must be validated before use to avoid sending the bearer token to an
// attacker-supplied host (SSRF / token exfiltration).
var graphAPIHosts = map[string]bool{
	"graph.microsoft.com":             true,
	"graph.microsoft.us":              true, // US Government L4
	"dod-graph.microsoft.us":          true, // US Government L5 (DOD)
	"microsoftgraph.chinacloudapi.cn": true, // 21Vianet (China)
}

// validateDeltaToken rejects a continuation token that isn't an HTTPS Microsoft
// Graph URL, so a model can't redirect an authenticated request elsewhere.
func validateDeltaToken(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid delta token")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("refusing non-HTTPS delta token")
	}
	if !graphAPIHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("refusing delta token for untrusted host %q", u.Hostname())
	}
	return nil
}

func isRemoved(additionalData map[string]any) bool {
	if additionalData == nil {
		return false
	}
	_, ok := additionalData["@removed"]
	return ok
}

// DeltaMessages returns message changes in a mail folder (default "inbox")
// since token. Pass an empty token to start a fresh sync.
func (c *Client) DeltaMessages(ctx context.Context, target, folderID, token string, top int32) ([]MailDelta, DeltaPage, error) {
	if folderID == "" {
		folderID = "inbox"
	}
	var resp users.ItemMailFoldersItemMessagesDeltaGetResponseable
	var err error
	if token == "" {
		cfg := &users.ItemMailFoldersItemMessagesDeltaRequestBuilderGetRequestConfiguration{Headers: maxPageSizeHeaders(top)}
		resp, err = c.targetUser(target).MailFolders().ByMailFolderId(folderID).Messages().Delta().GetAsDeltaGetResponse(ctx, cfg)
	} else {
		if err := validateDeltaToken(token); err != nil {
			return nil, DeltaPage{}, err
		}
		rb := users.NewItemMailFoldersItemMessagesDeltaRequestBuilder(token, c.inner.GetAdapter())
		resp, err = rb.GetAsDeltaGetResponse(ctx, nil)
	}
	if err != nil {
		return nil, DeltaPage{}, err
	}
	items := make([]MailDelta, 0, len(resp.GetValue()))
	for _, m := range resp.GetValue() {
		items = append(items, MailDelta{MailMessage: convertMessage(m), Removed: isRemoved(m.GetAdditionalData())})
	}
	return items, deltaPageFrom(resp), nil
}

// DeltaCalendarView returns event changes in [start, end] since token. The
// window is required for the initial sync; on resume the token already encodes
// it, so start/end are ignored. Pass an empty token to start a fresh sync.
func (c *Client) DeltaCalendarView(ctx context.Context, target, token string, start, end time.Time, top int32) ([]CalendarDelta, DeltaPage, error) {
	var resp users.ItemCalendarViewDeltaGetResponseable
	var err error
	if token == "" {
		s := start.UTC().Format(time.RFC3339)
		e := end.UTC().Format(time.RFC3339)
		qp := &users.ItemCalendarViewDeltaRequestBuilderGetQueryParameters{StartDateTime: &s, EndDateTime: &e}
		cfg := &users.ItemCalendarViewDeltaRequestBuilderGetRequestConfiguration{QueryParameters: qp, Headers: maxPageSizeHeaders(top)}
		resp, err = c.targetUser(target).CalendarView().Delta().GetAsDeltaGetResponse(ctx, cfg)
	} else {
		if err := validateDeltaToken(token); err != nil {
			return nil, DeltaPage{}, err
		}
		rb := users.NewItemCalendarViewDeltaRequestBuilder(token, c.inner.GetAdapter())
		resp, err = rb.GetAsDeltaGetResponse(ctx, nil)
	}
	if err != nil {
		return nil, DeltaPage{}, err
	}
	items := make([]CalendarDelta, 0, len(resp.GetValue()))
	for _, e := range resp.GetValue() {
		items = append(items, CalendarDelta{CalendarEvent: convertEvent(e), Removed: isRemoved(e.GetAdditionalData())})
	}
	return items, deltaPageFrom(resp), nil
}

// DeltaContacts returns contact changes since token. Pass an empty token to
// start a fresh sync.
func (c *Client) DeltaContacts(ctx context.Context, target, token string, top int32) ([]ContactDelta, DeltaPage, error) {
	var resp users.ItemContactsDeltaGetResponseable
	var err error
	if token == "" {
		cfg := &users.ItemContactsDeltaRequestBuilderGetRequestConfiguration{Headers: maxPageSizeHeaders(top)}
		resp, err = c.targetUser(target).Contacts().Delta().GetAsDeltaGetResponse(ctx, cfg)
	} else {
		if err := validateDeltaToken(token); err != nil {
			return nil, DeltaPage{}, err
		}
		rb := users.NewItemContactsDeltaRequestBuilder(token, c.inner.GetAdapter())
		resp, err = rb.GetAsDeltaGetResponse(ctx, nil)
	}
	if err != nil {
		return nil, DeltaPage{}, err
	}
	items := make([]ContactDelta, 0, len(resp.GetValue()))
	for _, ct := range resp.GetValue() {
		items = append(items, ContactDelta{Contact: convertContact(ct), Removed: isRemoved(ct.GetAdditionalData())})
	}
	return items, deltaPageFrom(resp), nil
}
