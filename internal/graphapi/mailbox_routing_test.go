package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// meBuilderPath is what the SDK's Me() builder emits before the Graph middleware
// rewrites it. The rewrite lives in the default pipeline (msgraph-sdk-go-core
// graph_client_factory.go ReplacementPairs), which the bare adapter these tests
// use does not install, so an own-mailbox request arrives here as
// /users/me-token-to-replace/... and reaches /me in production.
const meBuilderPath = "/v1.0/users/me-token-to-replace"

// The bug this pull request exists to fix was silent: a send with --mailbox set
// went out from the caller's own address and reported success, because the
// request still went to the caller's own mailbox. Guard and validation tests
// cannot see that — only the request path can, so these assert it directly, in
// both directions: a target must reach that mailbox, and no target must not
// reach some other one.
func TestDelegatedWritesAddressTheTargetMailbox(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
		call   func(*Client, context.Context, string) error
	}{
		{
			name:   "send from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/sendMail",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendMessage(ctx, target, &SendMessageOptions{
					Subject: "s", Body: "b", To: []string{"person@example.com"},
				})
			},
		},
		{
			name:   "send from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/sendMail",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendMessage(ctx, target, &SendMessageOptions{
					Subject: "s", Body: "b", To: []string{"person@example.com"},
				})
			},
		},
		{
			name:   "reply from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/reply",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", false)
			},
		},
		{
			name:   "reply-all from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/replyAll",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", true)
			},
		},
		{
			name:   "reply from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/reply",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", false)
			},
		},
		{
			name:   "forward from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/forward",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ForwardMessage(ctx, target, "AAA", "", []string{"person@example.com"})
			},
		},
		{
			name:   "forward from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/forward",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ForwardMessage(ctx, target, "AAA", "", []string{"person@example.com"})
			},
		},
		{
			name:   "reply-all from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/replyAll",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", true)
			},
		},
		{
			name:   "create a draft in the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.CreateDraft(ctx, target, "s", "b", []string{"person@example.com"}, nil, nil, false)
				return err
			},
		},
		{
			name:   "send a draft from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/send",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendDraft(ctx, target, "AAA")
			},
		},
		{
			name:   "create a draft in a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.CreateDraft(ctx, target, "s", "b", []string{"person@example.com"}, nil, nil, false)
				return err
			},
		},
		{
			name:   "send a draft that lives in a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/send",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendDraft(ctx, target, "AAA")
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
				// Draft creation is the one call here that reads its response.
				if strings.HasSuffix(req.URL.Path, "/messages") {
					return graphJSONResponse(req, `{"id":"draft-id","subject":"s"}`)
				}
				return graphEmptyResponse(req)
			})

			if err := tc.call(client, context.Background(), tc.target); err != nil {
				t.Fatalf("call: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph requests = %d, want 1", calls)
			}
			if got != tc.want {
				t.Errorf("request path = %q, want %q — a delegated write that lands on the "+
					"wrong path acts on the wrong mailbox and still reports success", got, tc.want)
			}
		})
	}
}
