package graphapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCreateReplyDraftAddsInlineImagesInOrder(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		replyAll     bool
		basePath     string
		createAction string
		attachments  []InlineAttachmentInput
	}{
		{
			name:         "own mailbox reply with one PNG",
			basePath:     meBuilderPath,
			createAction: "createReply",
			attachments: []InlineAttachmentInput{{
				ContentID: "hero", ContentType: "image/png", Name: "hero.png", Content: []byte("png-bytes"),
			}},
		},
		{
			name:         "delegated reply-all with multiple images",
			target:       "team@example.com",
			replyAll:     true,
			basePath:     "/v1.0/users/team@example.com",
			createAction: "createReplyAll",
			attachments: []InlineAttachmentInput{
				{ContentID: "first", ContentType: "image/png", Name: "first.png", Content: []byte("first-bytes")},
				{ContentID: "second", ContentType: "image/jpeg", Name: "second.jpg", Content: []byte("second-bytes")},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fragment := "<p>Images:"
			for _, attachment := range tc.attachments {
				fragment += `<img src="cid:` + attachment.ContentID + `">`
			}
			fragment += "</p>"

			var requests []string
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				requests = append(requests, req.Method+" "+req.URL.Path)
				switch len(requests) {
				case 1:
					assertReplyDraftRequest(t, req, http.MethodPost, tc.basePath+"/messages/AAA/"+tc.createAction)
					assertNoReplyContentPayload(t, req)
					return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject","body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
				case 2:
					assertReplyDraftRequest(t, req, http.MethodPatch, tc.basePath+"/messages/draft-id")
					wantBody := strings.Replace(generatedReplyHTML, `<body class="reply">`, `<body class="reply">`+fragment, 1)
					assertHTMLPatch(t, req, wantBody)
					return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
				default:
					attachmentIndex := len(requests) - 3
					if attachmentIndex < 0 || attachmentIndex >= len(tc.attachments) {
						t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
					}
					assertReplyDraftRequest(t, req, http.MethodPost, tc.basePath+"/messages/draft-id/attachments")
					assertInlineAttachmentPayload(t, req, tc.attachments[attachmentIndex])
					return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id"}`)
				}
			})

			draft, err := client.CreateReplyDraft(context.Background(), tc.target, "AAA", &CreateReplyDraftOptions{
				Body:              fragment,
				ReplyAll:          tc.replyAll,
				IsHTML:            true,
				InlineAttachments: tc.attachments,
			})
			if err != nil {
				t.Fatalf("CreateReplyDraft: %v", err)
			}
			if draft.ID != "draft-id" || !strings.Contains(draft.Body, "Original quoted history") {
				t.Errorf("draft = %#v, want ID and preserved quote", draft)
			}
			if len(requests) != 2+len(tc.attachments) {
				t.Fatalf("Graph requests = %d, want %d", len(requests), 2+len(tc.attachments))
			}
		})
	}
}

func TestCreateStandaloneDraftAddsMultipleInlineImages(t *testing.T) {
	attachments := []InlineAttachmentInput{
		{ContentID: "logo", Name: "logo.png", ContentType: "image/png", Content: []byte("logo")},
		{ContentID: "steps", Name: "steps.jpg", ContentType: "image/jpeg", Content: []byte("steps")},
	}
	body := `<p><img src="cid:logo"><img src="cid:steps"></p>`
	var requests []string
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests = append(requests, req.Method+" "+req.URL.Path)
		switch len(requests) {
		case 1:
			assertReplyDraftRequest(t, req, http.MethodPost, "/v1.0/users/team@example.com/messages")
			var payload struct {
				Body struct {
					ContentType string `json:"contentType"`
					Content     string `json:"content"`
				} `json:"body"`
				Attachments []json.RawMessage `json:"attachments"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode draft create request: %v", err)
			}
			if payload.Body.ContentType != "html" || payload.Body.Content != body {
				t.Errorf("draft body = %#v, want exact HTML", payload.Body)
			}
			if len(payload.Attachments) != 0 {
				t.Errorf("draft create embedded attachments before obtaining draft ID: %s", payload.Attachments)
			}
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Subject"}`)
		default:
			index := len(requests) - 2
			if index < 0 || index >= len(attachments) {
				t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			}
			assertReplyDraftRequest(t, req, http.MethodPost, "/v1.0/users/team@example.com/messages/draft-id/attachments")
			assertInlineAttachmentPayload(t, req, attachments[index])
			return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id"}`)
		}
	})

	draft, err := client.CreateDraft(
		context.Background(), "team@example.com", "Subject", body,
		[]string{"person@example.com"}, nil, nil, true, attachments...,
	)
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.ID != "draft-id" || len(requests) != 3 {
		t.Fatalf("draft = %#v requests = %v, want created draft plus two ordered attachments", draft, requests)
	}
}

func TestCreateStandaloneDraftInlineOwnMailboxIsAllowedByNoSend(t *testing.T) {
	attachment := InlineAttachmentInput{
		ContentID: "logo", Name: "logo.png", ContentType: "image/png", Content: []byte("logo"),
	}
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			assertReplyDraftRequest(t, req, http.MethodPost, meBuilderPath+"/messages")
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Subject"}`)
		case 2:
			assertReplyDraftRequest(t, req, http.MethodPost, meBuilderPath+"/messages/draft-id/attachments")
			assertInlineAttachmentPayload(t, req, attachment)
			return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id"}`)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphEmptyResponse(req)
		}
	})
	client.SetGuards(false, true)

	draft, err := client.CreateDraft(
		context.Background(), "", "Subject", `<img src="cid:logo">`,
		[]string{"person@example.com"}, nil, nil, true, attachment,
	)
	if err != nil {
		t.Fatalf("CreateDraft under --no-send: %v", err)
	}
	if draft.ID != "draft-id" || calls != 2 {
		t.Fatalf("draft = %#v calls = %d, want own-mailbox draft with one attachment", draft, calls)
	}
}

func TestCreateDraftValidatesInlineImagesBeforeGraph(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		t.Fatalf("unexpected Graph request during validation: %s %s", req.Method, req.URL.Path)
		return graphEmptyResponse(req)
	})
	_, err := client.CreateDraft(
		context.Background(), "", "Subject", `<p>No image</p>`,
		[]string{"person@example.com"}, nil, nil, true,
		InlineAttachmentInput{ContentID: "logo", Name: "logo.png", ContentType: "image/png", Content: []byte("logo")},
	)
	if err == nil || !strings.Contains(err.Error(), "does not reference") {
		t.Fatalf("CreateDraft error = %v, want unreferenced CID validation", err)
	}
	if calls != 0 {
		t.Fatalf("Graph requests = %d, want 0", calls)
	}
}

func TestCreateReplyDraftCleansUpAfterPostCreationFailure(t *testing.T) {
	tests := []struct {
		name       string
		attachment bool
		failCall   int
		wantAction string
	}{
		{name: "body patch failure", failCall: 2, wantAction: "formatting reply draft"},
		{name: "inline attachment failure", attachment: true, failCall: 3, wantAction: "adding inline attachment"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			deleted := false
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				if req.Method == http.MethodDelete {
					deleted = true
					assertReplyDraftRequest(t, req, http.MethodDelete, meBuilderPath+"/messages/draft-id")
					return replyDraftNoContentResponse(req)
				}
				if calls == 1 {
					return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject","body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
				}
				if calls == tc.failCall {
					return replyDraftErrorResponse(req, http.StatusInternalServerError, "MailboxStoreUnavailable", "Store unavailable")
				}
				return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
			})

			opts := &CreateReplyDraftOptions{Body: `<p>Hi</p>`, IsHTML: true}
			if tc.attachment {
				opts.Body = `<p><img src="cid:hero"></p>`
				opts.InlineAttachments = []InlineAttachmentInput{{ContentID: "hero", Name: "hero.png", ContentType: "image/png", Content: []byte("png")}}
			}
			_, err := client.CreateReplyDraft(context.Background(), "", "AAA", opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantAction) || !strings.Contains(err.Error(), "was deleted") {
				t.Fatalf("CreateReplyDraft error = %v, want action and successful cleanup", err)
			}
			if !deleted {
				t.Fatal("newly created draft was not deleted after failure")
			}
			if code, status := ErrorMetadata(err); code != "MailboxStoreUnavailable" || status != http.StatusInternalServerError {
				t.Errorf("ErrorMetadata = (%q, %d), want original Graph failure", code, status)
			}
		})
	}
}

func TestCreateStandaloneDraftCleansUpAfterInlineAttachmentFailure(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Subject"}`)
		case 2:
			return replyDraftErrorResponse(req, http.StatusInternalServerError, "AttachmentFailure", "Upload refused")
		case 3:
			assertReplyDraftRequest(t, req, http.MethodDelete, meBuilderPath+"/messages/draft-id")
			return replyDraftNoContentResponse(req)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphEmptyResponse(req)
		}
	})

	_, err := client.CreateDraft(
		context.Background(), "", "Subject", `<img src="cid:logo">`,
		[]string{"person@example.com"}, nil, nil, true,
		InlineAttachmentInput{ContentID: "logo", Name: "logo.png", ContentType: "image/png", Content: []byte("logo")},
	)
	if err == nil || !strings.Contains(err.Error(), "adding inline attachment") || !strings.Contains(err.Error(), "Upload refused") || !strings.Contains(err.Error(), "incomplete draft draft-id was deleted") {
		t.Fatalf("CreateDraft error = %v, want attachment failure and cleanup", err)
	}
}

func TestCreateReplyDraftCleanupFailureReportsSurvivingDraftAndPreservesCause(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject","body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
		case 2:
			return replyDraftErrorResponse(req, http.StatusForbidden, "OriginalFailure", "Patch refused")
		case 3:
			assertReplyDraftRequest(t, req, http.MethodDelete, meBuilderPath+"/messages/draft-id")
			return replyDraftErrorResponse(req, http.StatusForbidden, "CleanupDenied", "Delete refused")
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphEmptyResponse(req)
		}
	})

	_, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{Body: `<p>Hi</p>`, IsHTML: true})
	if err == nil {
		t.Fatal("CreateReplyDraft error = nil")
	}
	for _, want := range []string{"reply draft draft-id still exists", "CleanupDenied", "Delete refused"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q: %v", want, err)
		}
	}
	if code, status := ErrorMetadata(err); code != "OriginalFailure" || status != http.StatusForbidden {
		t.Errorf("ErrorMetadata = (%q, %d), want original failure metadata", code, status)
	}
}

func TestCreateReplyDraftFailsClosedWhenGeneratedBodyHasNoBodyElement(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch calls {
		case 1:
			return graphJSONResponse(req, `{"id":"draft-id","body":{"contentType":"html","content":"<div>Not a document</div>"}}`)
		case 2:
			response := graphJSONResponse(req, `{"id":"draft-id","body":{"contentType":"html","content":"<div>Still not a document</div>"}}`)
			response.Header.Set("Preference-Applied", `outlook.body-content-type="html"`)
			return response
		case 3:
			assertReplyDraftRequest(t, req, http.MethodDelete, meBuilderPath+"/messages/draft-id")
			return replyDraftNoContentResponse(req)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphEmptyResponse(req)
		}
	})

	_, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{Body: `<p>Hi</p>`, IsHTML: true})
	if err == nil || !strings.Contains(err.Error(), "usable HTML body") || !strings.Contains(err.Error(), "was deleted") {
		t.Fatalf("CreateReplyDraft error = %v, want fail-closed body error and cleanup", err)
	}
}

func TestHTMLBodyStartTagEnd(t *testing.T) {
	document := `<!doctype html><!-- <body>fake</body> --><html><head><style>.x:after{content:"<body>"}</style><script>const x = "<body>";</script></head><BODY data-note="1 > 0"><div>quote</div></BODY></html>`
	index, ok := htmlBodyStartTagEnd(document)
	if !ok {
		t.Fatal("htmlBodyStartTagEnd did not find body")
	}
	got := document[:index] + "<p>reply</p>" + document[index:]
	want := strings.Replace(document, `<BODY data-note="1 > 0">`, `<BODY data-note="1 > 0"><p>reply</p>`, 1)
	if got != want {
		t.Fatalf("inserted HTML = %q, want %q", got, want)
	}

	if _, ok := htmlBodyStartTagEnd(`<html><div>no body</div></html>`); ok {
		t.Fatal("htmlBodyStartTagEnd accepted document without body")
	}
}

func assertInlineAttachmentPayload(t *testing.T, req *http.Request, want InlineAttachmentInput) {
	t.Helper()
	var payload struct {
		ODataType    string `json:"@odata.type"`
		Name         string `json:"name"`
		ContentType  string `json:"contentType"`
		ContentBytes string `json:"contentBytes"`
		IsInline     bool   `json:"isInline"`
		ContentID    string `json:"contentId"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("decode attachment request: %v", err)
	}
	if payload.ODataType != "#microsoft.graph.fileAttachment" || payload.Name != want.Name ||
		payload.ContentType != want.ContentType || payload.ContentBytes != base64.StdEncoding.EncodeToString(want.Content) ||
		!payload.IsInline || payload.ContentID != want.ContentID {
		t.Fatalf("attachment payload = %#v, want CID %q MIME %q bytes %q inline fileAttachment", payload, want.ContentID, want.ContentType, want.Content)
	}
}
