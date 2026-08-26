package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMailAttachmentsCommandPreservesDelegatedTarget(t *testing.T) {
	tests := []struct {
		name      string
		args      func(string) []string
		wantPaths []string
		wantFile  bool
	}{
		{
			name: "list",
			args: func(_ string) []string {
				return []string{"message-id", "--mailbox", "shared@example.com", "--json"}
			},
			wantPaths: []string{"/v1.0/users/shared@example.com/messages/message-id/attachments"},
		},
		{
			name: "download all",
			args: func(out string) []string {
				return []string{"message-id", "--mailbox", "shared@example.com", "--save", "--out", out}
			},
			wantPaths: []string{
				"/v1.0/users/shared@example.com/messages/message-id/attachments",
				"/v1.0/users/shared@example.com/messages/message-id/attachments/attachment-id",
			},
			wantFile: true,
		},
		{
			name: "download one",
			args: func(out string) []string {
				return []string{"message-id", "--mailbox", "shared@example.com", "--attachment-id", "attachment-id", "--out", out}
			},
			wantPaths: []string{"/v1.0/users/shared@example.com/messages/message-id/attachments/attachment-id"},
			wantFile:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outDir := t.TempDir()
			var paths []string
			output, calls, err := runMailCommand(t, []string{"mail", "attachments"}, tc.args(outDir), func(req *http.Request) *http.Response {
				paths = append(paths, req.URL.Path)
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
				return graphJSONResponse(req, `{"value":[{
					"@odata.type":"#microsoft.graph.fileAttachment",
					"id":"attachment-id",
					"name":"file.txt",
					"contentType":"text/plain",
					"size":4
				}]}`)
			})
			if err != nil {
				t.Fatalf("mail attachments: %v", err)
			}
			if calls != len(tc.wantPaths) {
				t.Fatalf("Graph requests = %d, want %d", calls, len(tc.wantPaths))
			}
			if strings.Join(paths, "\n") != strings.Join(tc.wantPaths, "\n") {
				t.Fatalf("request paths = %v, want %v", paths, tc.wantPaths)
			}
			if tc.wantFile {
				content, err := os.ReadFile(filepath.Join(outDir, "file.txt"))
				if err != nil {
					t.Fatalf("read downloaded file: %v", err)
				}
				if string(content) != "test" {
					t.Errorf("downloaded content = %q, want test", content)
				}
				if !strings.Contains(output, "Saved:") {
					t.Errorf("output = %q, want Saved confirmation", output)
				}
			}
		})
	}
}

func TestMailAttachmentsCommandRejectsInvalidMailboxBeforeGraph(t *testing.T) {
	_, calls, err := runMailCommand(
		t,
		[]string{"mail", "attachments"},
		[]string{"message-id", "--mailbox", "not-an-address"},
		func(req *http.Request) *http.Response {
			t.Errorf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return graphJSONResponse(req, `{}`)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid --mailbox") {
		t.Fatalf("error = %v, want invalid --mailbox", err)
	}
	if calls != 0 {
		t.Fatalf("Graph requests = %d, want 0", calls)
	}
}
