package graphapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const generatedReplyHTML = `<html><head><style>.x{color:red}</style></head><body class="reply"><div id="quoted">Original quoted history</div></body></html>`

func TestCreateReplyDraftRoutesFormatsAndPreservesHistory(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		replyAll bool
		html     bool
		content  string
		basePath string
		action   string
	}{
		{
			name:     "plain reply in own mailbox",
			content:  "Thanks",
			basePath: meBuilderPath,
			action:   "createReply",
		},
		{
			name:     "plain reply-all in delegated mailbox",
			target:   "team@example.com",
			replyAll: true,
			content:  "Thanks all",
			basePath: "/v1.0/users/team@example.com",
			action:   "createReplyAll",
		},
		{
			name:     "HTML reply in own mailbox",
			html:     true,
			content:  "<p>Thanks</p>",
			basePath: meBuilderPath,
			action:   "createReply",
		},
		{
			name:     "HTML reply-all in delegated mailbox",
			target:   "team@example.com",
			replyAll: true,
			html:     true,
			content:  "<p>Thanks all</p>",
			basePath: "/v1.0/users/team@example.com",
			action:   "createReplyAll",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				switch calls {
				case 1:
					wantPath := tc.basePath + "/messages/AAA/" + tc.action
					assertReplyDraftRequest(t, req, http.MethodPost, wantPath)
					if tc.html {
						assertNoReplyContentPayload(t, req)
					} else {
						var payload replyActionPayload
						if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
							t.Fatalf("decode create reply request: %v", err)
						}
						if payload.Comment == nil || *payload.Comment != tc.content {
							t.Errorf("comment = %v, want exact %q", payload.Comment, tc.content)
						}
						if payload.Message != nil {
							t.Error("plain draft unexpectedly sent message.body")
						}
					}
					body := `{"id":"draft-id","subject":"Re: Original subject","toRecipients":[{"emailAddress":{"address":"person@example.com"}}]}`
					if tc.html {
						body = `{"id":"draft-id","subject":"Re: Original subject","toRecipients":[{"emailAddress":{"address":"person@example.com"}}],"body":{"contentType":"html","content":` + quotedJSON(generatedReplyHTML) + `}}`
					}
					return graphJSONResponse(req, body)
				case 2:
					if !tc.html {
						t.Fatalf("unexpected second request for plain draft: %s %s", req.Method, req.URL.Path)
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

			draft, err := client.CreateReplyDraft(context.Background(), tc.target, "AAA", &CreateReplyDraftOptions{
				Body:     tc.content,
				ReplyAll: tc.replyAll,
				IsHTML:   tc.html,
			})
			if err != nil {
				t.Fatalf("CreateReplyDraft: %v", err)
			}
			wantCalls := 1
			if tc.html {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("Graph requests = %d, want %d", calls, wantCalls)
			}
			if draft.ID != "draft-id" || draft.Subject != "Re: Original subject" {
				t.Errorf("draft = %#v, want returned Graph draft fields", draft)
			}
			if tc.html && (!strings.Contains(draft.Body, tc.content) || !strings.Contains(draft.Body, "Original quoted history")) {
				t.Errorf("draft body did not preserve reply and quote: %q", draft.Body)
			}
		})
	}
}

func TestCreateReplyDraftFetchesGeneratedHTMLWhenActionOmitsBody(t *testing.T) {
	wantBody := strings.Replace(generatedReplyHTML, `<body class="reply">`, `<body class="reply"><p>Thanks</p>`, 1)
	var requests []string
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests = append(requests, req.Method+" "+req.URL.Path)
		switch len(requests) {
		case 1:
			assertNoReplyContentPayload(t, req)
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
		case 2:
			if got := req.Header.Get("Prefer"); !strings.Contains(got, `outlook.body-content-type="html"`) {
				t.Errorf("GET Prefer header = %q, want HTML body preference", got)
			}
			response := graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject","body":{"contentType":"html","content":`+quotedJSON(generatedReplyHTML)+`}}`)
			response.Header.Set("Preference-Applied", `outlook.body-content-type="html"`)
			return response
		case 3:
			assertHTMLPatch(t, req, wantBody)
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphEmptyResponse(req)
		}
	})

	draft, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{
		Body:   "<p>Thanks</p>",
		IsHTML: true,
	})
	if err != nil {
		t.Fatalf("CreateReplyDraft: %v", err)
	}
	wantRequests := []string{
		"POST " + meBuilderPath + "/messages/AAA/createReply",
		"GET " + meBuilderPath + "/messages/draft-id",
		"PATCH " + meBuilderPath + "/messages/draft-id",
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("request order:\n%s\nwant:\n%s", strings.Join(requests, "\n"), strings.Join(wantRequests, "\n"))
	}
	if draft.Body != wantBody {
		t.Errorf("draft body = %q, want %q", draft.Body, wantBody)
	}
}

func TestCreateReplyDraftValidatesBeforeGraph(t *testing.T) {
	validAttachment := InlineAttachmentInput{
		ContentID: "image-one", Name: "image.png", ContentType: "image/png", Content: []byte("image"),
	}
	tests := []struct {
		name string
		opts *CreateReplyDraftOptions
		want string
	}{
		{name: "nil options", want: "options are required"},
		{name: "inline requires HTML", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, InlineAttachments: []InlineAttachmentInput{validAttachment}}, want: "require an HTML"},
		{name: "complete HTML document", opts: &CreateReplyDraftOptions{Body: `<html><body><p>Hi</p></body></html>`, IsHTML: true}, want: "must be a fragment"},
		{name: "malformed content ID", opts: &CreateReplyDraftOptions{Body: `<img src="cid:bad id">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{{ContentID: "bad id", Name: "x.png", ContentType: "image/png", Content: []byte("x")}}}, want: "invalid inline content ID"},
		{name: "duplicate content ID", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{validAttachment, {ContentID: "IMAGE-ONE", Name: "other.png", ContentType: "image/png", Content: []byte("other")}}}, want: "duplicate inline content ID"},
		{name: "missing filename", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{{ContentID: "image-one", ContentType: "image/png", Content: []byte("x")}}}, want: "has no filename"},
		{name: "non-image MIME", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{{ContentID: "image-one", Name: "x.txt", ContentType: "text/plain", Content: []byte("x")}}}, want: "non-image content type"},
		{name: "empty attachment", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{{ContentID: "image-one", Name: "x.png", ContentType: "image/png"}}}, want: "is empty"},
		{name: "simple attachment limit is exclusive", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{{ContentID: "image-one", Name: "x.png", ContentType: "image/png", Content: make([]byte, MaxInlineAttachmentBytes)}}}, want: "must be under 3 MB"},
		{name: "unreferenced content ID", opts: &CreateReplyDraftOptions{Body: `<p>No image</p>`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{validAttachment}}, want: "does not reference"},
		{name: "content ID prefix is not an exact reference", opts: &CreateReplyDraftOptions{Body: `<img src="cid:image-one-large">`, IsHTML: true, InlineAttachments: []InlineAttachmentInput{validAttachment}}, want: "does not reference"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				t.Fatalf("unexpected Graph request during validation: %s %s", req.Method, req.URL.Path)
				return graphEmptyResponse(req)
			})
			_, err := client.CreateReplyDraft(context.Background(), "", "AAA", tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CreateReplyDraft error = %v, want containing %q", err, tc.want)
			}
			if calls != 0 {
				t.Fatalf("Graph requests = %d, want 0", calls)
			}
		})
	}
}

func TestCreateReplyDraftCapabilityGuards(t *testing.T) {
	ctx := context.Background()
	opts := &CreateReplyDraftOptions{Body: "Thanks"}

	noWrite := &Client{noWrite: true}
	if _, err := noWrite.CreateReplyDraft(ctx, "team@example.com", "AAA", opts); !errors.Is(err, ErrNoWrite) {
		t.Fatalf("CreateReplyDraft under --no-write = %v, want ErrNoWrite", err)
	}

	calls := 0
	noSend := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
	})
	noSend.SetGuards(false, true)
	if _, err := noSend.CreateReplyDraft(ctx, "", "AAA", opts); err != nil {
		t.Fatalf("CreateReplyDraft under --no-send: %v", err)
	}
	if calls != 1 {
		t.Fatalf("CreateReplyDraft Graph requests under --no-send = %d, want 1", calls)
	}

	if err := noSend.ReplyMessage(ctx, "", "AAA", "Thanks", false, false); !errors.Is(err, ErrNoSend) {
		t.Fatalf("immediate ReplyMessage under --no-send = %v, want ErrNoSend", err)
	}
}

func TestCreateReplyDraftRejectsInvalidIDBeforeGraph(t *testing.T) {
	client := &Client{}
	_, err := client.CreateReplyDraft(context.Background(), "team@example.com", "", &CreateReplyDraftOptions{Body: "Thanks"})
	if err == nil || !strings.Contains(err.Error(), "message ID") {
		t.Fatalf("CreateReplyDraft invalid ID error = %v, want message ID validation", err)
	}
}

func TestCreateReplyDraftDelegatedErrors(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		status     int
		message    string
		want       string
		wantAbsent []string
	}{
		{name: "permission refusal gets draft-specific guidance", code: "ErrorAccessDenied", status: http.StatusForbidden, message: "Access is denied.", want: "does not require Mail.Send.Shared, Send As, or Send on Behalf Of"},
		{name: "not found gets neutral mailbox and ID guidance", code: "ErrorItemNotFound", status: http.StatusNotFound, message: "The item could not be found.", want: "mailbox is unavailable or Full Access is missing", wantAbsent: []string{}},
		{name: "throttle gets no permission or ID guidance", code: "TooManyRequests", status: http.StatusTooManyRequests, message: "Too many requests.", want: "creating reply draft in team@example.com", wantAbsent: []string{"Mail.ReadWrite.Shared", "Full Access", "IDs are scoped to a mailbox"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				return replyDraftErrorResponse(req, tc.status, tc.code, tc.message)
			})

			_, err := client.CreateReplyDraft(context.Background(), "team@example.com", "AAA", &CreateReplyDraftOptions{Body: "Thanks"})
			if err == nil {
				t.Fatal("CreateReplyDraft error = nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not contain %q:\n%s", tc.want, err)
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(err.Error(), absent) {
					t.Errorf("error unexpectedly contains %q:\n%s", absent, err)
				}
			}
			if code, status := ErrorMetadata(err); code != tc.code || status != tc.status {
				t.Errorf("ErrorMetadata = (%q, %d), want (%q, %d)", code, status, tc.code, tc.status)
			}
		})
	}
}

func assertReplyDraftRequest(t *testing.T, req *http.Request, method, path string) {
	t.Helper()
	if req.Method != method || req.URL.Path != path {
		t.Fatalf("request = %s %s, want %s %s", req.Method, req.URL.Path, method, path)
	}
}

func assertNoReplyContentPayload(t *testing.T, req *http.Request) {
	t.Helper()
	if req.Body == nil {
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read create reply request: %v", err)
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode create reply request %q: %v", body, err)
	}
	if _, exists := payload["comment"]; exists {
		t.Errorf("HTML draft create unexpectedly supplied comment: %s", body)
	}
	if _, exists := payload["message"]; exists {
		t.Errorf("HTML draft create unexpectedly supplied message body: %s", body)
	}
}

func assertHTMLPatch(t *testing.T, req *http.Request, want string) {
	t.Helper()
	var payload struct {
		Body *struct {
			ContentType string `json:"contentType"`
			Content     string `json:"content"`
		} `json:"body"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatalf("decode patch request: %v", err)
	}
	if payload.Body == nil || payload.Body.ContentType != "html" || payload.Body.Content != want {
		t.Fatalf("patch body = %#v, want exact preserved HTML %q", payload.Body, want)
	}
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func replyDraftErrorResponse(req *http.Request, status int, code, message string) *http.Response {
	body := `{"error":{"code":` + quotedJSON(code) + `,"message":` + quotedJSON(message) + `}}`
	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		Request:       req,
		ContentLength: int64(len(body)),
	}
}

func replyDraftNoContentResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Status:     http.StatusText(http.StatusNoContent),
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}
}
