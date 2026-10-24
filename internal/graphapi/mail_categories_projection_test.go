package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The category value was observed on a controlled real message. IDs and routes
// are sanitized; this models provider selection, not a captured HTTP response.
func TestMessageDetailReadsPreserveSelectedCategories(t *testing.T) {
	for _, target := range []string{"", "shared@example.com"} {
		for _, batch := range []bool{false, true} {
			for _, categorized := range []bool{true, false} {
				name := "get/own/empty"
				if batch {
					name = "batch/own/empty"
				}
				if target != "" {
					name = strings.Replace(name, "own", "delegated", 1)
				}
				if categorized {
					name = strings.Replace(name, "empty", "categorized", 1)
				}
				t.Run(name, func(t *testing.T) {
					const messageID = "categorized-message"
					want := []string{}
					if categorized {
						want = []string{"Olkcli category test"}
					}
					client := testGraphClient(t, func(req *http.Request) *http.Response {
						message := func(endpoint *url.URL) map[string]any {
							wantPath := "/me/messages/" + messageID
							if !batch {
								// The test transport bypasses the SDK middleware that
								// rewrites the signed-in-user placeholder to /me.
								wantPath = "/users/me-token-to-replace/messages/" + messageID
							}
							if target != "" {
								wantPath = "/users/" + target + "/messages/" + messageID
							}
							if got := strings.TrimPrefix(endpoint.Path, "/v1.0"); got != wantPath {
								t.Fatalf("message path = %q, want %q", got, wantPath)
							}
							result := map[string]any{"id": messageID}
							if slices.Contains(strings.Split(endpoint.Query().Get("$select"), ","), "categories") {
								result["categories"] = want
							}
							return result
						}
						var payload any
						if batch {
							var request struct {
								Requests []struct {
									ID  string `json:"id"`
									URL string `json:"url"`
								} `json:"requests"`
							}
							if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
								t.Fatal(err)
							}
							if len(request.Requests) != 1 {
								t.Fatalf("batch size = %d, want 1", len(request.Requests))
							}
							item := request.Requests[0]
							endpoint, err := url.Parse(item.URL)
							if err != nil {
								t.Fatal(err)
							}
							payload = map[string]any{"responses": []any{map[string]any{
								"id": item.ID, "status": http.StatusOK,
								"headers": map[string]string{"Content-Type": "application/json"},
								"body":    message(endpoint),
							}}}
						} else {
							payload = message(req.URL)
						}
						data, err := json.Marshal(payload)
						if err != nil {
							t.Fatal(err)
						}
						return graphJSONResponse(req, string(data))
					})
					var got *MailMessage
					if batch {
						messages, err := client.GetMessagesBatch(context.Background(), target, []string{messageID}, MessageBodyDefault)
						if err != nil {
							t.Fatal(err)
						}
						if len(messages) != 1 {
							t.Fatalf("messages = %d, want 1", len(messages))
						}
						got = &messages[0]
					} else {
						var err error
						got, err = client.GetMessage(context.Background(), target, messageID, MessageBodyDefault)
						if err != nil {
							t.Fatal(err)
						}
					}
					if !slices.Equal(got.Categories, want) {
						t.Errorf("categories = %v, want %v", got.Categories, want)
					}
					data, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					var output map[string]json.RawMessage
					if err := json.Unmarshal(data, &output); err != nil {
						t.Fatal(err)
					}
					raw, present := output["categories"]
					if present != categorized {
						t.Fatalf("JSON categories present = %v, want %v", present, categorized)
					}
					if categorized {
						var values []string
						if err := json.Unmarshal(raw, &values); err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(values, want) {
							t.Errorf("JSON categories = %v, want %v", values, want)
						}
					}
				})
			}
		}
	}
}
