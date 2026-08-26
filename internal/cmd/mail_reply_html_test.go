package cmd

import (
	"net/http"
	"testing"
)

func TestMailReplyAndForwardHTMLFlagsReachGraph(t *testing.T) {
	tests := []struct {
		name string
		path []string
		args []string
	}{
		{
			name: "reply",
			path: []string{"mail", "reply"},
			args: []string{"message-id", "--body", "<p>Reply</p>", "--html"},
		},
		{
			name: "forward",
			path: []string{"mail", "forward"},
			args: []string{"message-id", "--to", "person@example.com", "--comment", "<p>Forward</p>", "--html"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, calls, err := runMailCommand(t, tc.path, tc.args, func(req *http.Request) *http.Response {
				var payload struct {
					Comment *string `json:"comment"`
					Message *struct {
						Body *struct {
							ContentType string `json:"contentType"`
						} `json:"body"`
					} `json:"message"`
				}
				if err := decodeGraphJSON(req.Body, &payload); err != nil {
					t.Fatalf("decode Graph request: %v", err)
				}
				if payload.Comment != nil {
					t.Errorf("--html request sent plain comment %q", *payload.Comment)
				}
				if payload.Message == nil || payload.Message.Body == nil || payload.Message.Body.ContentType != "html" {
					t.Errorf("--html message body = %#v, want HTML content type", payload.Message)
				}
				return graphJSONResponse(req, "")
			})
			if err != nil {
				t.Fatalf("command: %v", err)
			}
			if calls != 1 {
				t.Errorf("Graph requests = %d, want 1", calls)
			}
			if output == "" {
				t.Error("command omitted success output")
			}
		})
	}
}
