package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type replyRecipientPayload struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type replyActionPayload struct {
	Comment *string `json:"comment"`
	Message *struct {
		Body *struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		ToRecipients []replyRecipientPayload `json:"toRecipients"`
	} `json:"message"`
	ToRecipients []replyRecipientPayload `json:"toRecipients"`
}

func TestReplyAndForwardBodyFormats(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		html        bool
		wantForward bool
		call        func(*Client) error
	}{
		{
			name: "plain reply keeps comment payload",
			path: "/reply",
			call: func(c *Client) error {
				return c.ReplyMessage(context.Background(), "", "message-id", "Reply body", false, false)
			},
		},
		{
			name: "HTML reply sends typed message body",
			path: "/reply",
			html: true,
			call: func(c *Client) error {
				return c.ReplyMessage(context.Background(), "", "message-id", "<p>Reply body</p>", false, true)
			},
		},
		{
			name: "HTML reply-all sends typed message body",
			path: "/replyAll",
			html: true,
			call: func(c *Client) error {
				return c.ReplyMessage(context.Background(), "", "message-id", "<p>Reply body</p>", true, true)
			},
		},
		{
			name:        "plain forward keeps comment and recipient payload",
			path:        "/forward",
			wantForward: true,
			call: func(c *Client) error {
				return c.ForwardMessage(context.Background(), "", "message-id", "Reply body", []string{"person@example.com"}, false)
			},
		},
		{
			name:        "HTML forward sends body and recipients in message payload",
			path:        "/forward",
			html:        true,
			wantForward: true,
			call: func(c *Client) error {
				return c.ForwardMessage(context.Background(), "", "message-id", "<p>Reply body</p>", []string{"person@example.com"}, true)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload replyActionPayload
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				if !strings.HasSuffix(req.URL.Path, tc.path) {
					t.Errorf("request path = %q, want suffix %q", req.URL.Path, tc.path)
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request body: %v", err)
				}
				return graphEmptyResponse(req)
			})

			if err := tc.call(client); err != nil {
				t.Fatalf("mail action: %v", err)
			}

			if tc.html {
				if payload.Comment != nil {
					t.Errorf("HTML action sent comment %q alongside message body", *payload.Comment)
				}
				if payload.Message == nil || payload.Message.Body == nil {
					t.Fatal("HTML action omitted message body")
				}
				if got := payload.Message.Body.ContentType; got != "html" {
					t.Errorf("content type = %q, want html", got)
				}
				if got := payload.Message.Body.Content; got != "<p>Reply body</p>" {
					t.Errorf("body content = %q, want HTML input", got)
				}
			} else {
				if payload.Comment == nil || *payload.Comment != "Reply body" {
					t.Errorf("plain comment = %v, want Reply body", payload.Comment)
				}
				if payload.Message != nil {
					t.Error("plain action unexpectedly sent a message payload")
				}
			}

			if tc.wantForward {
				recipients := payload.ToRecipients
				if tc.html {
					recipients = payload.Message.ToRecipients
					if len(payload.ToRecipients) != 0 {
						t.Error("HTML forward sent recipients outside the message payload")
					}
				}
				if len(recipients) != 1 || recipients[0].EmailAddress.Address != "person@example.com" {
					t.Errorf("forward recipients = %#v, want person@example.com", recipients)
				}
			}
		})
	}
}
