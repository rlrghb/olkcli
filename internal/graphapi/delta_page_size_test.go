package graphapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDeltaPageSizePreference(t *testing.T) {
	ctx := context.Background()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, resource := range []struct {
		name string
		path string
		call func(*Client, string, string, int32) (DeltaPage, bool, error)
	}{
		{"mail", "/mailFolders/inbox/messages/delta", func(c *Client, target, token string, top int32) (DeltaPage, bool, error) {
			items, page, err := c.DeltaMessages(ctx, target, "inbox", token, top)
			return page, len(items) == 1 && items[0].ID == "removed-item" && items[0].Removed, err
		}},
		{"calendar", "/calendarView/delta", func(c *Client, target, token string, top int32) (DeltaPage, bool, error) {
			items, page, err := c.DeltaCalendarView(ctx, target, token, start, start.AddDate(0, 1, 0), top)
			return page, len(items) == 1 && items[0].ID == "removed-item" && items[0].Removed, err
		}},
		{"contacts", "/contacts/delta", func(c *Client, target, token string, top int32) (DeltaPage, bool, error) {
			items, page, err := c.DeltaContacts(ctx, target, token, top)
			return page, len(items) == 1 && items[0].ID == "removed-item" && items[0].Removed, err
		}},
	} {
		for _, target := range []string{"", "shared@example.com"} {
			for _, top := range []int32{500, 0, -1} {
				for _, stage := range []string{"initial", "nextLink", "deltaLink"} {
					t.Run(fmt.Sprintf("%s/%s/top=%d/%s", resource.name, target, top, stage), func(t *testing.T) {
						userPath := "/me"
						initialUserPath := "/users/me-token-to-replace"
						if target != "" {
							userPath = "/users/" + target
							initialUserPath = userPath
						}
						base := "https://graph.microsoft.com/v1.0" + userPath + resource.path
						token := ""
						switch stage {
						case "nextLink":
							token = base + "?$skiptoken=opaque%2Bvalue%2F%3D&custom=keep%20this"
						case "deltaLink":
							token = base + "?$deltatoken=opaque%2Bvalue%2F%3D"
						}
						linkType := "@odata.nextLink"
						complete := stage == "deltaLink"
						if complete {
							linkType = "@odata.deltaLink"
						}
						returnedToken := base + "?$skiptoken=returned"
						if complete {
							returnedToken = base + "?$deltatoken=returned"
						}
						requests := 0
						client := testGraphClient(t, func(req *http.Request) *http.Response {
							requests++
							preference := strings.Join(req.Header.Values("Prefer"), ",")
							want := ""
							if top > 0 {
								want = fmt.Sprintf("odata.maxpagesize=%d", top)
							}
							if preference != want {
								t.Errorf("Prefer = %q, want %q", preference, want)
							}
							if token != "" {
								if got := req.URL.String(); got != token {
									t.Errorf("continuation URL = %q, want %q", got, token)
								}
							} else if wantPath := "/v1.0" + initialUserPath + resource.path + "()"; req.URL.Path != wantPath {
								t.Errorf("initial path = %q, want %q", req.URL.Path, wantPath)
							}
							if req.URL.Query().Has("$top") {
								t.Error("request unexpectedly includes $top")
							}
							return graphJSONResponse(req, fmt.Sprintf(`{"value":[{"id":"removed-item","@removed":{"reason":"deleted"}}],%q:%q}`, linkType, returnedToken))
						})
						page, removed, err := resource.call(client, target, token, top)
						if err != nil {
							t.Fatal(err)
						}
						if requests != 1 || !removed {
							t.Errorf("requests = %d, tombstone preserved = %t", requests, removed)
						}
						if page.Token != returnedToken || page.Complete != complete {
							t.Errorf("page = %+v, want token %q and complete %t", page, returnedToken, complete)
						}
					})
				}
			}
		}
	}
}
