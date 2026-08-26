package cmd

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMailDraftsCreateWiresMultipleInlineImages(t *testing.T) {
	logo := writeTestFile(t, "logo.png", testPNG)
	steps := writeTestFile(t, "steps.jpg", append([]byte{0xff, 0xd8, 0xff, 0xdb}, make([]byte, 20)...))
	body := `<p><img src="cid:logo"><img src="cid:steps"></p>`
	call := 0

	output, calls, err := runMailCommand(t, []string{"mail", "drafts", "create"}, []string{
		"--to", "person@example.com", "--subject", "Instructions", "--body", body, "--html",
		"--inline", "logo=" + logo, "--inline", "steps=" + steps,
		"--mailbox", "team@example.com",
	}, func(req *http.Request) *http.Response {
		call++
		switch call {
		case 1:
			if req.Method != http.MethodPost || req.URL.Path != "/v1.0/users/team@example.com/messages" {
				t.Fatalf("create request = %s %s", req.Method, req.URL.Path)
			}
			var payload struct {
				Body struct {
					ContentType string `json:"contentType"`
					Content     string `json:"content"`
				} `json:"body"`
				Attachments []any `json:"attachments"`
			}
			if err := decodeGraphJSON(req.Body, &payload); err != nil {
				t.Fatalf("decode draft create request: %v", err)
			}
			if payload.Body.ContentType != "html" || payload.Body.Content != body || len(payload.Attachments) != 0 {
				t.Fatalf("draft payload = %#v, want exact HTML and no embedded attachments", payload)
			}
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Instructions"}`)
		case 2, 3:
			if req.Method != http.MethodPost || req.URL.Path != "/v1.0/users/team@example.com/messages/draft-id/attachments" {
				t.Fatalf("attachment request = %s %s", req.Method, req.URL.Path)
			}
			var payload struct {
				ContentID string `json:"contentId"`
				IsInline  bool   `json:"isInline"`
			}
			if err := decodeGraphJSON(req.Body, &payload); err != nil {
				t.Fatalf("decode attachment: %v", err)
			}
			wantCID := "logo"
			if call == 3 {
				wantCID = "steps"
			}
			if payload.ContentID != wantCID || !payload.IsInline {
				t.Fatalf("attachment payload = %#v, want inline CID %q", payload, wantCID)
			}
			return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id"}`)
		default:
			t.Fatalf("unexpected Graph request %d: %s %s", call, req.Method, req.URL.Path)
			return graphJSONResponse(req, `{}`)
		}
	})
	if err != nil {
		t.Fatalf("mail drafts create --html --inline: %v", err)
	}
	if calls != 3 {
		t.Fatalf("Graph requests = %d, want 3", calls)
	}
	wantOutput := "Draft created in team@example.com: Instructions (ID: draft-id)\n"
	if output != wantOutput {
		t.Fatalf("output = %q, want %q", output, wantOutput)
	}
}

func TestMailDraftsCreateInlineDryRunAndValidation(t *testing.T) {
	image := writeTestFile(t, "tile.png", testPNG)
	body := `<p><img src="cid:tile"></p>`

	output, calls, err := runMailCommand(t, []string{"mail", "drafts", "create"}, []string{
		"--to", "person@example.com", "--subject", "Instructions", "--body", body,
		"--html", "--inline", "tile=" + image, "--dry-run",
	}, func(req *http.Request) *http.Response {
		t.Fatalf("unexpected Graph request during dry run: %s %s", req.Method, req.URL.Path)
		return graphJSONResponse(req, `{}`)
	})
	if err != nil {
		t.Fatalf("mail drafts create --dry-run: %v", err)
	}
	want := "Would create draft:\n  In: your own mailbox\n  To: person@example.com\n  Subject: Instructions\n  Body: " + body + "\n  Inline image: tile=tile.png (16 bytes)\n"
	if calls != 0 || output != want {
		t.Fatalf("dry run calls = %d output = %q, want zero calls and %q", calls, output, want)
	}
	if strings.Contains(output, string(testPNG)) {
		t.Fatal("dry run printed image contents")
	}

	_, calls, err = runMailCommand(t, []string{"mail", "drafts", "create"}, []string{
		"--to", "person@example.com", "--subject", "Instructions", "--body", body,
		"--inline", "tile=" + image, "--dry-run",
	}, func(req *http.Request) *http.Response {
		body, _ := io.ReadAll(req.Body)
		t.Fatalf("unexpected Graph request during validation: %s %s %s", req.Method, req.URL.Path, body)
		return graphJSONResponse(req, `{}`)
	})
	if err == nil || !strings.Contains(err.Error(), "--inline requires --html") {
		t.Fatalf("validation error = %v, want --html requirement", err)
	}
	if calls != 0 {
		t.Fatalf("validation Graph requests = %d, want 0", calls)
	}
}
