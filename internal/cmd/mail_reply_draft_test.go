package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rlrghb/olkcli/internal/graphapi"
)

var testPNG = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}

func TestMailReplyDraftCommandPlainRoutesAndReportsDraft(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPath    string
		wantComment string
		wantOutput  string
	}{
		{
			name:        "plain reply in own mailbox",
			args:        []string{"AAA", "--body", "Thanks", "--draft"},
			wantPath:    "/v1.0/me/messages/AAA/createReply",
			wantComment: "Thanks",
			wantOutput:  "Reply draft created in your own mailbox: Re: Original subject (ID: draft-id)\n",
		},
		{
			name:        "plain reply-all in delegated mailbox",
			args:        []string{"AAA", "--body", "Thanks all", "--reply-all", "--draft", "--mailbox", "team@example.com"},
			wantPath:    "/v1.0/users/team@example.com/messages/AAA/createReplyAll",
			wantComment: "Thanks all",
			wantOutput:  "Reply-all draft created in team@example.com: Re: Original subject (ID: draft-id)\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, calls, err := runMailCommand(t, []string{"mail", "reply"}, tc.args, func(req *http.Request) *http.Response {
				if req.Method != http.MethodPost || req.URL.Path != tc.wantPath {
					t.Fatalf("request = %s %s, want POST %s", req.Method, req.URL.Path, tc.wantPath)
				}
				var payload struct {
					Comment *string `json:"comment"`
					Message any     `json:"message"`
				}
				if err := decodeGraphJSON(req.Body, &payload); err != nil {
					t.Fatalf("decode Graph request: %v", err)
				}
				if payload.Comment == nil || *payload.Comment != tc.wantComment || payload.Message != nil {
					t.Fatalf("plain reply payload = %#v, want exact comment only", payload)
				}
				return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Original subject"}`)
			})
			if err != nil {
				t.Fatalf("mail reply --draft: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph requests = %d, want 1", calls)
			}
			if output != tc.wantOutput {
				t.Fatalf("output = %q, want %q", output, tc.wantOutput)
			}
			if strings.Contains(output, "sent") {
				t.Fatalf("draft output used sent-success wording: %q", output)
			}
		})
	}
}

func TestMailReplyDraftCommandWiresThreadedHTMLAndMultipleInlineImages(t *testing.T) {
	logo := writeTestFile(t, "logo.png", testPNG)
	steps := writeTestFile(t, "steps.jpg", append([]byte{0xff, 0xd8, 0xff, 0xdb}, make([]byte, 20)...))
	body := `<p><img src="cid:logo"><img src="cid:steps"></p>`
	generated := `<html><body><div id="quote">Original history</div></body></html>`
	wantCombined := `<html><body>` + body + `<div id="quote">Original history</div></body></html>`
	call := 0

	output, calls, err := runMailCommand(t, []string{"mail", "reply"}, []string{
		"AAA", "--body", body, "--reply-all", "--html", "--draft",
		"--inline", "logo=" + logo, "--inline", "steps=" + steps,
		"--mailbox", "team@example.com",
	}, func(req *http.Request) *http.Response {
		call++
		switch call {
		case 1:
			if req.Method != http.MethodPost || req.URL.Path != "/v1.0/users/team@example.com/messages/AAA/createReplyAll" {
				t.Fatalf("create request = %s %s", req.Method, req.URL.Path)
			}
			if req.Body != nil {
				payload, readErr := io.ReadAll(req.Body)
				if readErr != nil {
					t.Fatalf("read create request: %v", readErr)
				}
				if trimmed := strings.TrimSpace(string(payload)); trimmed != "" && trimmed != "{}" && trimmed != "null" {
					t.Fatalf("HTML create supplied replacement body: %q", payload)
				}
			}
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Original subject","body":{"contentType":"html","content":`+mustJSONQuote(t, generated)+`}}`)
		case 2:
			if req.Method != http.MethodPatch || req.URL.Path != "/v1.0/users/team@example.com/messages/draft-id" {
				t.Fatalf("patch request = %s %s", req.Method, req.URL.Path)
			}
			var payload struct {
				Body struct {
					ContentType string `json:"contentType"`
					Content     string `json:"content"`
				} `json:"body"`
			}
			if err := decodeGraphJSON(req.Body, &payload); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			if payload.Body.ContentType != "html" || payload.Body.Content != wantCombined {
				t.Fatalf("patch body = %#v, want formatted reply plus quote", payload.Body)
			}
			return graphJSONResponse(req, `{"id":"draft-id"}`)
		case 3, 4:
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
			if call == 4 {
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
		t.Fatalf("mail reply --html --draft --inline: %v", err)
	}
	if calls != 4 {
		t.Fatalf("Graph requests = %d, want 4", calls)
	}
	wantOutput := "Reply-all draft created in team@example.com: Re: Original subject (ID: draft-id)\n"
	if output != wantOutput {
		t.Fatalf("output = %q, want %q", output, wantOutput)
	}
}

func TestMailReplyDraftDryRunExactOutputAndZeroGraphRequests(t *testing.T) {
	image := writeTestFile(t, "tile.png", testPNG)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "plain reply draft",
			args: []string{"AAA", "--body", "Thanks", "--draft", "--dry-run"},
			want: "Would create reply draft for message AAA in your own mailbox\n",
		},
		{
			name: "HTML reply draft",
			args: []string{"AAA", "--body", "<p>Thanks</p>", "--html", "--draft", "--dry-run"},
			want: "Would create reply draft for message AAA in your own mailbox\n",
		},
		{
			name: "delegated HTML reply-all draft with inline image",
			args: []string{"AAA", "--body", `<p><img src="cid:tile"></p>`, "--reply-all", "--html", "--draft", "--inline", "tile=" + image, "--mailbox", "team@example.com", "--dry-run"},
			want: "Would create reply-all draft for message AAA in team@example.com\n  Inline image: tile=tile.png (16 bytes)\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, calls, err := runMailCommand(t, []string{"mail", "reply"}, tc.args, func(req *http.Request) *http.Response {
				t.Errorf("unexpected Graph request during dry run: %s %s", req.Method, req.URL.Path)
				return graphJSONResponse(req, `{}`)
			})
			if err != nil {
				t.Fatalf("mail reply --draft --dry-run: %v", err)
			}
			if calls != 0 {
				t.Fatalf("Graph requests = %d, want 0", calls)
			}
			if output != tc.want {
				t.Fatalf("output = %q, want %q", output, tc.want)
			}
			if strings.Contains(output, string(testPNG)) {
				t.Fatal("dry run printed image contents")
			}
		})
	}
}

func TestMailReplyInlineValidationBeforeGraph(t *testing.T) {
	image := writeTestFile(t, "image.png", testPNG)
	textFile := writeTestFile(t, "not-image.txt", []byte("plain text"))
	oversized := writeTestFile(t, "large.png", make([]byte, graphapi.MaxInlineAttachmentBytes))
	directory := t.TempDir()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "requires HTML", args: []string{"AAA", "--body", `<img src="cid:x">`, "--draft", "--inline", "x=" + image, "--dry-run"}, want: "requires both --html and --draft"},
		{name: "requires draft", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--inline", "x=" + image, "--dry-run"}, want: "requires both --html and --draft"},
		{name: "malformed mapping", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x", "--dry-run"}, want: "expected CID=PATH"},
		{name: "unsafe CID", args: []string{"AAA", "--body", `<img src="cid:bad id">`, "--html", "--draft", "--inline", "bad id=" + image, "--dry-run"}, want: "invalid inline content ID"},
		{name: "duplicate CID", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x=" + image, "--inline", "X=" + image, "--dry-run"}, want: "duplicate inline content ID"},
		{name: "missing file", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x=/definitely/missing/image.png", "--dry-run"}, want: "no such file"},
		{name: "directory", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x=" + directory, "--dry-run"}, want: "not a regular file"},
		{name: "non-image file", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x=" + textFile, "--dry-run"}, want: "non-image content type"},
		{name: "unreferenced CID", args: []string{"AAA", "--body", `<p>No image</p>`, "--html", "--draft", "--inline", "x=" + image, "--dry-run"}, want: "does not reference"},
		{name: "oversized image", args: []string{"AAA", "--body", `<img src="cid:x">`, "--html", "--draft", "--inline", "x=" + oversized, "--dry-run"}, want: "must be under 3 MB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, calls, err := runMailCommand(t, []string{"mail", "reply"}, tc.args, func(req *http.Request) *http.Response {
				t.Fatalf("unexpected Graph request during validation: %s %s", req.Method, req.URL.Path)
				return graphJSONResponse(req, `{}`)
			})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("command error = %v, want containing %q", err, tc.want)
			}
			if calls != 0 {
				t.Fatalf("Graph requests = %d, want 0", calls)
			}
		})
	}
}

func TestMailReplyDraftCommandCapabilityGuards(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   error
		wantCalls int
	}{
		{name: "no-write blocks draft creation", args: []string{"AAA", "--body", "Thanks", "--draft", "--no-write"}, wantErr: graphapi.ErrNoWrite},
		{name: "no-send allows draft creation", args: []string{"AAA", "--body", "Thanks", "--draft", "--no-send"}, wantCalls: 1},
		{name: "no-send still blocks immediate reply", args: []string{"AAA", "--body", "Thanks", "--no-send"}, wantErr: graphapi.ErrNoSend},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, calls, err := runGuardedMailReplyCommand(t, tc.args, func(req *http.Request) *http.Response {
				return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Subject"}`)
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("command error = %v, want %v", err, tc.wantErr)
			}
			if calls != tc.wantCalls {
				t.Fatalf("Graph requests = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

func runGuardedMailReplyCommand(
	t *testing.T,
	args []string,
	responder func(*http.Request) *http.Response,
) (output string, calls int, err error) {
	t.Helper()
	client := testMailListClient(t, func(req *http.Request) *http.Response {
		calls++
		return responder(req)
	})
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		return "", calls, err
	}
	kctx, err := parser.Parse(append([]string{"mail", "reply"}, args...))
	if err != nil {
		return "", calls, err
	}
	client.SetGuards(cli.NoWrite, cli.NoSend)
	output, _, err = captureStd(func() error {
		return kctx.Run(&RunContext{Ctx: context.Background(), Flags: &cli.RootFlags, client: client})
	})
	return output, calls, err
}

func writeTestFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON string: %v", err)
	}
	return string(encoded)
}
