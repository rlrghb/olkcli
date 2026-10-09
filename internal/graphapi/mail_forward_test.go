package graphapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type forwardPayload struct {
	Comment *string `json:"comment"`
	Message *struct {
		Body *struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
		ToRecipients []replyRecipientPayload `json:"toRecipients"`
		CcRecipients []replyRecipientPayload `json:"ccRecipients"`
	} `json:"message"`
	ToRecipients []replyRecipientPayload `json:"toRecipients"`
}

func recipientList(recipients []replyRecipientPayload) string {
	addresses := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		addresses = append(addresses, recipient.EmailAddress.Address)
	}
	return strings.Join(addresses, ",")
}

func decodeForwardPayload(t *testing.T, req *http.Request) forwardPayload {
	t.Helper()
	var payload forwardPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("decode forward request: %v", err)
	}
	return payload
}

func TestForwardMessageCarriesCcInsideTheMessage(t *testing.T) {
	for _, html := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "HTML"}[html], func(t *testing.T) {
			var payload forwardPayload
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				if !strings.HasSuffix(req.URL.Path, "/messages/AAA/forward") {
					t.Errorf("request path = %q, want the forward action", req.URL.Path)
				}
				payload = decodeForwardPayload(t, req)
				return graphEmptyResponse(req)
			})
			err := client.ForwardMessage(context.Background(), "team@example.com", "AAA", &ForwardOptions{
				To: []string{"person@example.com"}, Cc: []string{"copied@example.com"}, Comment: "See below", IsHTML: html,
			})
			if err != nil {
				t.Fatalf("ForwardMessage: %v", err)
			}
			if payload.Message == nil {
				t.Fatal("forward with Cc omitted the message payload")
			}
			if got := recipientList(payload.Message.ToRecipients); got != "person@example.com" {
				t.Errorf("message.toRecipients = %q, want person@example.com", got)
			}
			if got := recipientList(payload.Message.CcRecipients); got != "copied@example.com" {
				t.Errorf("message.ccRecipients = %q, want copied@example.com", got)
			}
			if len(payload.ToRecipients) != 0 {
				t.Error("forward sent To recipients outside the message payload as well")
			}
			if html {
				if payload.Comment != nil || payload.Message.Body == nil || payload.Message.Body.Content != "See below" {
					t.Errorf("HTML forward payload = %+v, want the comment as message body only", payload)
				}
			} else if payload.Comment == nil || *payload.Comment != "<div>See below</div>" || payload.Message.Body != nil {
				t.Errorf("plain forward payload = %+v, want the comment field and no message body", payload)
			}
		})
	}
}

func TestCreateForwardDraftRoutesAndKeepsTheForwardedOriginal(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		html     bool
		content  string
		basePath string
	}{
		{name: "plain draft in own mailbox", content: "See below", basePath: meBuilderPath},
		{name: "plain draft in delegated mailbox", target: "team@example.com", content: "See below", basePath: "/v1.0/users/team@example.com"},
		{name: "HTML draft in own mailbox", html: true, content: "<p>See below</p>", basePath: meBuilderPath},
		{name: "HTML draft in delegated mailbox", target: "team@example.com", html: true, content: "<p>See below</p>", basePath: "/v1.0/users/team@example.com"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				switch calls {
				case 1:
					assertReplyDraftRequest(t, req, http.MethodPost, tc.basePath+"/messages/AAA/createForward")
					payload := decodeForwardPayload(t, req)
					if payload.Message == nil ||
						recipientList(payload.Message.ToRecipients) != "person@example.com" ||
						recipientList(payload.Message.CcRecipients) != "copied@example.com" {
						t.Errorf("createForward recipients = %+v, want To and Cc inside the message", payload.Message)
					}
					if payload.Message != nil && payload.Message.Body != nil {
						t.Error("createForward sent a message body, which would replace the forwarded original")
					}
					if tc.html && payload.Comment != nil {
						t.Errorf("HTML forward draft sent comment %q; the fragment belongs in the generated body", *payload.Comment)
					}
					if !tc.html && (payload.Comment == nil || *payload.Comment != plainTextHTML(tc.content)) {
						t.Errorf("plain forward draft comment = %v, want %q", payload.Comment, tc.content)
					}
					return graphJSONResponse(req, `{"id":"draft-id","subject":"FW: Original subject",`+
						`"toRecipients":[{"emailAddress":{"address":"person@example.com"}}],`+
						`"body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
				case 2:
					if !tc.html {
						t.Fatalf("unexpected second request for a plain forward draft: %s %s", req.Method, req.URL.Path)
					}
					assertReplyDraftRequest(t, req, http.MethodPatch, tc.basePath+"/messages/draft-id")
					wantBody := strings.Replace(generatedReplyHTML, `<body class="reply">`, `<body class="reply">`+tc.content, 1)
					assertHTMLPatch(t, req, wantBody)
					return graphJSONResponse(req, `{"id":"draft-id","body":{"contentType":"html","content":`+quotedJSON(wantBody)+`}}`)
				default:
					t.Fatalf("unexpected Graph request %d: %s %s", calls, req.Method, req.URL.Path)
					return graphEmptyResponse(req)
				}
			})

			draft, err := client.CreateForwardDraft(context.Background(), tc.target, "AAA", &ForwardOptions{
				To: []string{"person@example.com"}, Cc: []string{"copied@example.com"}, Comment: tc.content, IsHTML: tc.html,
			})
			if err != nil {
				t.Fatalf("CreateForwardDraft: %v", err)
			}
			if wantCalls := map[bool]int{false: 1, true: 2}[tc.html]; calls != wantCalls {
				t.Fatalf("Graph requests = %d, want %d", calls, wantCalls)
			}
			if draft.ID != "draft-id" || draft.Subject != "FW: Original subject" {
				t.Errorf("draft = %#v, want the Graph draft's ID and subject", draft)
			}
			if tc.html && (!strings.Contains(draft.Body, tc.content) || !strings.Contains(draft.Body, "Original quoted history")) {
				t.Errorf("draft body lost the comment or the forwarded original: %q", draft.Body)
			}
		})
	}
}

func TestCreateForwardDraftGuardsAndValidation(t *testing.T) {
	opts := &ForwardOptions{To: []string{"person@example.com"}, Comment: "See below"}
	ctx := context.Background()

	noWrite := &Client{noWrite: true}
	if _, err := noWrite.CreateForwardDraft(ctx, "team@example.com", "AAA", opts); !errors.Is(err, ErrNoWrite) {
		t.Errorf("--no-write error = %v, want ErrNoWrite", err)
	}

	noSend := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{"id":"draft-id","subject":"FW: Original subject"}`)
	})
	noSend.SetGuards(false, true)
	if _, err := noSend.CreateForwardDraft(ctx, "", "AAA", opts); err != nil {
		t.Errorf("--no-send blocked a forward draft, which sends nothing: %v", err)
	}

	unused := testGraphClient(t, func(req *http.Request) *http.Response {
		t.Fatalf("invalid input reached Graph: %s %s", req.Method, req.URL.Path)
		return nil
	})
	for name, bad := range map[string]*ForwardOptions{
		"invalid cc":             {To: []string{"person@example.com"}, Cc: []string{"not-an-address"}},
		"complete HTML document": {To: []string{"person@example.com"}, Comment: "<html><body>Hi</body></html>", IsHTML: true},
		"missing options":        nil,
	} {
		if _, err := unused.CreateForwardDraft(ctx, "", "AAA", bad); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCreateForwardDraftCleanupNamesAForwardDraft(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch req.Method {
		case http.MethodPost:
			return graphJSONResponse(req, `{"id":"draft-id","subject":"FW: Original subject","body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
		case http.MethodPatch:
			response := graphJSONResponse(req, `{"error":{"code":"ErrorAccessDenied","message":"Access is denied"}}`)
			response.StatusCode = http.StatusForbidden
			return response
		case http.MethodDelete:
			return graphEmptyResponse(req)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return nil
		}
	})
	_, err := client.CreateForwardDraft(context.Background(), "team@example.com", "AAA", &ForwardOptions{
		To: []string{"person@example.com"}, Comment: "<p>See below</p>", IsHTML: true,
	})
	if err == nil {
		t.Fatal("expected the failed body patch to be reported")
	}
	for _, want := range []string{"formatting forward draft", "incomplete forward draft draft-id was deleted"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
	if calls != 3 {
		t.Errorf("Graph requests = %d, want create, patch and delete", calls)
	}
}
