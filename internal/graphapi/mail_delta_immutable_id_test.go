package graphapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMailDeltaImmutableIDs(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, target := range []string{"", "shared@example.com"} {
			for _, top := range []int32{0, 500} {
				for _, stage := range []string{"initial", "nextLink", "deltaLink"} {
					t.Run(fmt.Sprintf("enabled=%t/%s/top=%d/%s", enabled, target, top, stage), func(t *testing.T) {
						userPath := "/me"
						initialUserPath := "/users/me-token-to-replace"
						if target != "" {
							userPath = "/users/" + target
							initialUserPath = userPath
						}
						path := "/mailFolders/inbox/messages/delta"
						base := "https://graph.microsoft.com/v1.0" + userPath + path
						token := ""
						switch stage {
						case "nextLink":
							token = base + "?$skiptoken=opaque%2Bvalue%2F%3D&custom=keep%20this"
						case "deltaLink":
							token = base + "?$deltatoken=opaque%2Bvalue%2F%3D"
						}
						complete := stage == "deltaLink"
						linkType := "@odata.nextLink"
						returnedToken := base + "?$skiptoken=returned"
						if complete {
							linkType = "@odata.deltaLink"
							returnedToken = base + "?$deltatoken=returned"
						}
						requests := 0
						client := testGraphClient(t, func(req *http.Request) *http.Response {
							requests++
							preference := strings.Join(req.Header.Values("Prefer"), ",")
							if got := strings.Contains(preference, `IdType="ImmutableId"`); got != enabled {
								t.Errorf("Prefer = %q, want immutable IDs enabled = %t", preference, enabled)
							}
							if got := strings.Contains(preference, "odata.maxpagesize=500"); got != (top > 0) {
								t.Errorf("Prefer = %q, want page-size preference enabled = %t", preference, top > 0)
							}
							if token != "" {
								if got := req.URL.String(); got != token {
									t.Errorf("continuation URL = %q, want %q", got, token)
								}
							} else {
								if wantPath := "/v1.0" + initialUserPath + path + "()"; req.URL.Path != wantPath {
									t.Errorf("initial path = %q, want %q", req.URL.Path, wantPath)
								}
							}
							return graphJSONResponse(req, fmt.Sprintf(`{"value":[{"id":"immutable-message"},{"id":"removed-message","@removed":{"reason":"deleted"}}],%q:%q}`, linkType, returnedToken))
						})
						client.SetImmutableIDs(enabled)
						items, page, err := client.DeltaMessages(context.Background(), target, "inbox", token, top)
						if err != nil {
							t.Fatal(err)
						}
						if requests != 1 {
							t.Errorf("requests = %d, want 1", requests)
						}
						if len(items) != 2 || items[0].ID != "immutable-message" || items[0].Removed || items[1].ID != "removed-message" || !items[1].Removed {
							t.Errorf("items = %+v, want unchanged message IDs and tombstone", items)
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
