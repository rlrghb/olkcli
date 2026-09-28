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

// Honor $select like the provider: a conversion-only fixture that always
// supplies replyTo can hide a missing field in the outbound request.
func TestMessageDetailReadsPreserveSelectedReplyTo(t *testing.T) {
	for _, target := range []string{"", "shared@example.com"} {
		for _, batch := range []bool{false, true} {
			name := "get/" + target
			if batch {
				name = "batch/" + target
			}
			t.Run(name, func(t *testing.T) {
				client := testGraphClient(t, func(req *http.Request) *http.Response {
					message := func(query url.Values) map[string]any {
						result := map[string]any{"id": "message-id"}
						if slices.Contains(strings.Split(query.Get("$select"), ","), "replyTo") {
							result["replyTo"] = []any{map[string]any{"emailAddress": map[string]string{"address": "reply@example.com"}}}
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
							"body":    message(endpoint.Query()),
						}}}
					} else {
						payload = message(req.URL.Query())
					}
					data, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					return graphJSONResponse(req, string(data))
				})
				var got []string
				if batch {
					messages, err := client.GetMessagesBatch(context.Background(), target, []string{"message-id"}, MessageBodyDefault)
					if err != nil {
						t.Fatal(err)
					}
					if len(messages) != 1 {
						t.Fatalf("messages = %d, want 1", len(messages))
					}
					got = messages[0].ReplyTo
				} else {
					message, err := client.GetMessage(context.Background(), target, "message-id", MessageBodyDefault)
					if err != nil {
						t.Fatal(err)
					}
					got = message.ReplyTo
				}
				if want := []string{"reply@example.com"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("replyTo = %v, want %v", got, want)
				}
			})
		}
	}
}
