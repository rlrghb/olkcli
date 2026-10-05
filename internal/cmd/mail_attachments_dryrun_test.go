package cmd

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rlrghb/olkcli/internal/config"
	"github.com/rlrghb/olkcli/internal/secrets"
)

func TestMailAttachmentsDryRunDoesNotDownloadOrCreateDirectory(t *testing.T) {
	for _, single := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			outDir := t.TempDir()
			if !existing {
				outDir = filepath.Join(outDir, "new", "downloads")
			}
			args := []string{"msg-1", "--dry-run", "--out", outDir}
			want := "Would save attachments from message msg-1"
			if single {
				args = append(args, "--attachment-id", "att-1")
				want = "Would download attachment att-1 from message msg-1"
			} else {
				args = append(args, "--save")
			}
			out, calls, err := runMailCommand(t, []string{"mail", "attachments"}, args,
				func(req *http.Request) *http.Response {
					return graphJSONResponse(req, `{"value":[{"@odata.type":"#microsoft.graph.fileAttachment","id":"att-1","name":"file.txt","size":4}],"@odata.type":"#microsoft.graph.fileAttachment","id":"att-1","name":"file.txt","contentBytes":"dGVzdA=="}`)
				})
			if err != nil || calls != 0 || !strings.HasPrefix(out, want) {
				t.Errorf("single=%v existing=%v output=%q calls=%d error=%v", single, existing, out, calls, err)
			}
			if existing {
				entries, err := os.ReadDir(outDir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("directory changed: entries=%v error=%v", entries, err)
				}
			} else if _, err := os.Stat(filepath.Dir(outDir)); !os.IsNotExist(err) {
				t.Fatalf("dry-run created output parent: error=%v", err)
			}
		}
	}
}

func TestMailAttachmentsDryRunWithoutAccount(t *testing.T) {
	for _, args := range [][]string{
		{"mail", "attachments", "msg-1", "--save", "--dry-run"},
		{"mail", "attachments", "msg-1", "--attachment-id", "att-1", "--dry-run"},
	} {
		cli := &CLI{}
		parser, err := newKongParser(cli)
		if err != nil {
			t.Fatal(err)
		}
		kctx, err := parser.Parse(args)
		if err != nil {
			t.Fatal(err)
		}
		// An empty configuration and inert store make GraphClient fail without
		// accessing the host's configuration, keychain, or network.
		ctx := &RunContext{Ctx: context.Background(), Flags: &cli.RootFlags,
			cfg: &config.Config{}, store: &secrets.KeyringStore{}}
		_, _, err = captureStd(func() error { return kctx.Run(ctx) })
		if err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}
