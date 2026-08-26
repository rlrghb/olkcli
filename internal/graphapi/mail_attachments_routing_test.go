package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestMailAttachmentsAddressTheTargetMailbox(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
		call   func(*Client, context.Context, string) error
	}{
		{
			name:   "list from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/message-id/attachments",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.GetAttachments(ctx, target, "message-id")
				return err
			},
		},
		{
			name:   "list from a delegated mailbox",
			target: "shared@example.com",
			want:   "/v1.0/users/shared@example.com/messages/message-id/attachments",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.GetAttachments(ctx, target, "message-id")
				return err
			},
		},
		{
			name:   "download from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/message-id/attachments/attachment-id",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.DownloadAttachment(ctx, target, "message-id", "attachment-id")
				return err
			},
		},
		{
			name:   "download from a delegated mailbox",
			target: "shared@example.com",
			want:   "/v1.0/users/shared@example.com/messages/message-id/attachments/attachment-id",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.DownloadAttachment(ctx, target, "message-id", "attachment-id")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			calls := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				got = req.URL.Path
				if strings.HasSuffix(req.URL.Path, "/attachment-id") {
					return graphJSONResponse(req, `{
						"@odata.type":"#microsoft.graph.fileAttachment",
						"id":"attachment-id",
						"name":"file.txt",
						"contentType":"text/plain",
						"size":4,
						"contentBytes":"dGVzdA=="
					}`)
				}
				return graphJSONResponse(req, `{"value":[]}`)
			})

			if err := tc.call(client, context.Background(), tc.target); err != nil {
				t.Fatalf("call: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph requests = %d, want 1", calls)
			}
			if got != tc.want {
				t.Errorf("request path = %q, want %q", got, tc.want)
			}
		})
	}
}
