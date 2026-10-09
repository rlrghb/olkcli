package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPlainTextHTMLKeepsLinesBlankLinesAndIndentation(t *testing.T) {
	tests := map[string]struct {
		text string
		want string
	}{
		"empty":                 {"", ""},
		"only line terminators": {"\n\r\n", "<div><br></div><div><br></div>"},
		"single line":           {"Thanks", "<div>Thanks</div>"},
		"paragraphs": {
			"First paragraph\nstill first\n\nSecond paragraph",
			"<div>First paragraph</div><div>still first</div><div><br></div><div>Second paragraph</div>",
		},
		"CRLF and bare CR":    {"one\r\ntwo\rthree", "<div>one</div><div>two</div><div>three</div>"},
		"one line terminator": {"end\n", "<div>end</div>"},
		"trailing blank line": {"end\n\n", "<div>end</div><div><br></div>"},
		"trailing space":      {"end ", "<div>end&nbsp;</div>"},
		"leading blank line":  {"\nafter", "<div><br></div><div>after</div>"},
		"indented log excerpt": {
			"Log:\n    2026-09-29 ERROR  tunnel down",
			"<div>Log:</div><div>&nbsp;&nbsp;&nbsp;&nbsp;2026-09-29 ERROR &nbsp;tunnel down</div>",
		},
		"tab indentation":      {"\tkey: value", "<div>&nbsp;&nbsp;&nbsp;&nbsp;key: value</div>"},
		"whitespace-only line": {"a\n  \nb", "<div>a</div><div>&nbsp;&nbsp;</div><div>b</div>"},
		"markup is escaped": {
			`if a < b && c > "d" then <b>x</b>`,
			"<div>if a &lt; b &amp;&amp; c &gt; &#34;d&#34; then &lt;b&gt;x&lt;/b&gt;</div>",
		},
		"non-ASCII kept": {"Grüße — ok", "<div>Grüße — ok</div>"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := plainTextHTML(tc.text); got != tc.want {
				t.Errorf("plainTextHTML(%q)\n got %q\nwant %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestPlainReplyAndForwardSendRenderedCommentWithoutReadingOriginal(t *testing.T) {
	const text = "Hello,\n\nSee below:\n  step 1 < step 2"
	want := plainTextHTML(text)
	tests := []struct {
		name   string
		target string
		base   string
		action string
		call   func(*Client, string) error
	}{
		{"reply", "", meBuilderPath, "/reply", func(c *Client, target string) error {
			return c.ReplyMessage(context.Background(), target, "AAA", &ReplyOptions{Body: text})
		}},
		{"reply-all from a shared mailbox", "team@example.com", "/v1.0/users/team@example.com", "/replyAll",
			func(c *Client, target string) error {
				return c.ReplyMessage(context.Background(), target, "AAA", &ReplyOptions{Body: text, ReplyAll: true})
			}},
		{"forward", "", meBuilderPath, "/forward", func(c *Client, target string) error {
			return c.ForwardMessage(context.Background(), target, "AAA", &ForwardOptions{To: []string{"person@example.com"}, Comment: text})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload replyActionPayload
			actions := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				actions++
				if req.Method != http.MethodPost || req.URL.Path != tc.base+"/messages/AAA"+tc.action {
					t.Fatalf("request = %s %s, want POST %s", req.Method, req.URL.Path, tc.base+"/messages/AAA"+tc.action)
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				return graphEmptyResponse(req)
			})
			if err := tc.call(client, tc.target); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if actions != 1 {
				t.Fatalf("action requests = %d, want 1", actions)
			}
			if payload.Comment == nil || *payload.Comment != want {
				t.Errorf("comment = %v, want rendered HTML %q", payload.Comment, want)
			}
			if payload.Message != nil && payload.Message.Body != nil {
				t.Error("plain action sent message.body alongside the comment")
			}
		})
	}
}

func TestPlainReplyDraftCreatesWithRenderedCommentWithoutReadingOriginal(t *testing.T) {
	var comment *string
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		var payload replyActionPayload
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		comment = payload.Comment
		if payload.Message != nil && payload.Message.Body != nil {
			t.Error("plain draft sent message.body alongside the comment")
		}
		return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Original subject"}`)
	})
	_, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{Body: "one\ntwo"})
	if err != nil {
		t.Fatalf("CreateReplyDraft: %v", err)
	}
	if comment == nil || *comment != "<div>one</div><div>two</div>" {
		t.Errorf("comment = %v, want each line as a div", comment)
	}
}

type recipientPatchPayload struct {
	Body *struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
	CcRecipients  []replyRecipientPayload `json:"ccRecipients"`
	BccRecipients []replyRecipientPayload `json:"bccRecipients"`
}

func TestReplyDraftAddsCcAndBccToTheGeneratedRecipients(t *testing.T) {
	for _, html := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "HTML"}[html], func(t *testing.T) {
			var patch recipientPatchPayload
			patches := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				switch req.Method {
				case http.MethodPost:
					body := `{"id":"draft-id","subject":"Re: Original subject",` + generatedReplyRecipients
					if html {
						body += `,"body":{"contentType":"html","content":` + quotedJSON(generatedReplyHTML) + `}`
					}
					return graphJSONResponse(req, body+`}`)
				case http.MethodPatch:
					patches++
					if err := json.NewDecoder(req.Body).Decode(&patch); err != nil {
						t.Fatalf("decode patch: %v", err)
					}
					return graphJSONResponse(req, `{"id":"draft-id"}`)
				default:
					t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
					return nil
				}
			})
			draft, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{
				Body: "Thanks", ReplyAll: true, IsHTML: html,
				Cc:  []string{"added@example.com", "COPIED@example.com", "Person@example.com", "added@example.com"},
				Bcc: []string{"audit@example.com", "ADDED@example.com"},
			})
			if err != nil {
				t.Fatalf("CreateReplyDraft: %v", err)
			}
			if patches != 1 {
				t.Fatalf("PATCH requests = %d, want one carrying recipients", patches)
			}
			wantCc := "copied@example.com,second@example.com,added@example.com"
			wantBcc := "hidden@example.com,audit@example.com"
			if got := recipientList(patch.CcRecipients); got != wantCc {
				t.Errorf("patched cc = %s, want %s", got, wantCc)
			}
			if got := recipientList(patch.BccRecipients); got != wantBcc {
				t.Errorf("patched bcc = %s, want %s", got, wantBcc)
			}
			if html != (patch.Body != nil) {
				t.Errorf("patch body present = %v, want %v", patch.Body != nil, html)
			}
			if strings.Join(draft.Cc, ",") != wantCc || strings.Join(draft.Bcc, ",") != wantBcc {
				t.Errorf("reported cc %v bcc %v, want the merged lists", draft.Cc, draft.Bcc)
			}
		})
	}
}

func TestReplyDraftRefusesToReplaceAnUnreportedCcList(t *testing.T) {
	deleted := false
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		switch req.Method {
		case http.MethodPost:
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Original subject"}`)
		case http.MethodDelete:
			deleted = true
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
			return nil
		}
	})
	_, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{
		Body: "Thanks", ReplyAll: true, Cc: []string{"added@example.com"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be extended safely") {
		t.Fatalf("error = %v, want a refusal to replace an unknown Cc list", err)
	}
	if !deleted {
		t.Error("the incomplete draft was not deleted")
	}
}

func TestReplyRejectsAnInvalidAddedRecipientBeforeAnyRequest(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		return nil
	})
	err := client.ReplyMessage(context.Background(), "", "AAA", &ReplyOptions{Body: "x", Cc: []string{"not-an-address"}})
	if err == nil {
		t.Fatal("ReplyMessage accepted an invalid Cc address")
	}
}

func TestReplyWithAddedRecipientsSendsThroughADraft(t *testing.T) {
	for _, sendFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "sent", true: "send fails"}[sendFails], func(t *testing.T) {
			var steps []string
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				steps = append(steps, req.Method+" "+req.URL.Path[strings.LastIndex(req.URL.Path, "/"):])
				switch {
				case strings.HasSuffix(req.URL.Path, "/replyAll") || strings.HasSuffix(req.URL.Path, "/reply"):
					t.Fatalf("reply with added recipients used the direct action %s", req.URL.Path)
				case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/createReplyAll"):
					return graphJSONResponse(req, `{"id":"draft-id",`+generatedReplyRecipients+`}`)
				case req.Method == http.MethodPatch:
					return graphJSONResponse(req, `{"id":"draft-id"}`)
				case req.Method == http.MethodGet:
					return graphJSONResponse(req, `{"isDraft":true}`)
				case strings.HasSuffix(req.URL.Path, "/send") && sendFails:
					return replyDraftErrorResponse(req, http.StatusForbidden, "ErrorAccessDenied", "Access is denied.")
				case strings.HasSuffix(req.URL.Path, "/send"):
					return &http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody, Request: req}
				case req.Method == http.MethodDelete:
					return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}
				}
				t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
				return nil
			})
			err := client.ReplyMessage(context.Background(), "", "AAA", &ReplyOptions{
				Body: "Thanks", ReplyAll: true, Cc: []string{"added@example.com"},
			})
			want := "POST /createReplyAll,PATCH /draft-id,GET /draft-id,POST /send"
			if sendFails {
				want += ",DELETE /draft-id"
				if err == nil || !strings.Contains(err.Error(), "Access is denied") || !strings.Contains(err.Error(), "was deleted") {
					t.Errorf("error = %v, want the send failure and the draft cleanup", err)
				}
			} else if err != nil {
				t.Fatalf("ReplyMessage: %v", err)
			}
			if got := strings.Join(steps, ","); got != want {
				t.Errorf("requests = %s, want %s", got, want)
			}
		})
	}
}

func TestForwardCarriesBcc(t *testing.T) {
	var payload struct {
		Message *struct {
			BccRecipients []replyRecipientPayload `json:"bccRecipients"`
		} `json:"message"`
	}
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode forward: %v", err)
		}
		return graphEmptyResponse(req)
	})
	err := client.ForwardMessage(context.Background(), "", "AAA", &ForwardOptions{
		To: []string{"person@example.com"}, Bcc: []string{"audit@example.com"}, Comment: "FYI",
	})
	if err != nil {
		t.Fatalf("ForwardMessage: %v", err)
	}
	if payload.Message == nil || recipientList(payload.Message.BccRecipients) != "audit@example.com" {
		t.Errorf("forward payload = %+v, want the Bcc inside the message", payload.Message)
	}
}

func TestImmediateHTMLReplyWithRecipientsAcceptsFullDocument(t *testing.T) {
	const document = `<!doctype html><html><head><style>p {color: red}</style></head><body><p>Hello</p></body></html>`
	for _, all := range []bool{false, true} {
		for _, target := range []string{"", "team@example.com"} {
			base := meBuilderPath
			if target != "" {
				base = "/v1.0/users/" + target
			}
			action := "/createReply"
			if all {
				action += "All"
			}
			var steps []string
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				steps = append(steps, req.Method)
				switch req.Method {
				case http.MethodPost:
					if req.URL.Path == base+"/messages/AAA"+action {
						var payload replyActionPayload
						if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
							t.Fatal(err)
						}
						if payload.Comment != nil || payload.Message == nil || payload.Message.Body == nil || payload.Message.Body.Content != document || !strings.EqualFold(payload.Message.Body.ContentType, "html") {
							t.Fatalf("HTML document was not preserved: %+v", payload)
						}
						return graphJSONResponse(req, `{"id":"draft-id",`+generatedReplyRecipients+`}`)
					}
					if req.URL.Path != base+"/messages/draft-id/send" {
						t.Fatalf("unexpected send path %s", req.URL.Path)
					}
					return graphEmptyResponse(req)
				case http.MethodPatch:
					var payload recipientPatchPayload
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload.Body != nil || !strings.Contains(recipientList(payload.CcRecipients), "extra@example.com") || !strings.Contains(recipientList(payload.BccRecipients), "audit@example.com") {
						t.Fatalf("unexpected recipient patch: %+v", payload)
					}
					return graphJSONResponse(req, `{"id":"draft-id"}`)
				case http.MethodGet:
					return graphJSONResponse(req, `{"isDraft":true}`)
				default:
					t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
					return nil
				}
			})
			if err := client.ReplyMessage(context.Background(), target, "AAA", &ReplyOptions{Body: document, IsHTML: true, ReplyAll: all, Cc: []string{"extra@example.com"}, Bcc: []string{"audit@example.com"}}); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(steps, ","); got != "POST,PATCH,GET,POST" {
				t.Fatalf("requests = %s", got)
			}
		}
	}
}
