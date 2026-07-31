package cmd

import (
	"encoding/json"
	"net/http"
	"path"
	"reflect"
	"strings"
	"testing"
)

func TestMailFoldersWellKnownResolvesCanonicalGraphFolders(t *testing.T) {
	wantNames := []string{"archive", "deleteditems", "inbox", "junkemail"}
	for _, wantName := range wantNames {
		t.Run(wantName, func(t *testing.T) {
			output, calls, err := runMailCommand(
				t,
				[]string{"mail", "folders", "list"},
				[]string{"--json", "--well-known", wantName},
				func(req *http.Request) *http.Response {
					if got := path.Base(req.URL.Path); got != wantName {
						t.Errorf("resolved Graph folder name = %q, want %q", got, wantName)
					}
					return graphJSONResponse(req, `{
						"id":"folder-`+wantName+`",
						"displayName":"Untrusted display name",
						"totalItemCount":12,
						"unreadItemCount":3,
						"parentFolderId":"root"
					}`)
				},
			)
			if err != nil {
				t.Fatalf("mail folders list --well-known %s: %v", wantName, err)
			}
			if calls != 1 {
				t.Fatalf("Graph handler calls = %d, want 1", calls)
			}

			var envelope struct {
				Results []struct {
					ID            string `json:"id"`
					WellKnownName string `json:"wellKnownName"`
				} `json:"results"`
				Count int `json:"count"`
			}
			if err := json.Unmarshal([]byte(output), &envelope); err != nil {
				t.Fatalf("decode JSON output: %v\n%s", err, output)
			}
			if envelope.Count != 1 || len(envelope.Results) != 1 {
				t.Fatalf("result shape = count %d, rows %d, want 1", envelope.Count, len(envelope.Results))
			}
			if got := envelope.Results[0]; got.ID != "folder-"+wantName || got.WellKnownName != wantName {
				t.Fatalf("folder result = %#v, want canonical %q mapping", got, wantName)
			}
		})
	}
}

func TestMailFoldersWellKnownRejectsUnknownNameWithoutGraphRequest(t *testing.T) {
	_, calls, err := runMailCommand(
		t,
		[]string{"mail", "folders", "list"},
		[]string{"--json", "--well-known", "Archive"},
		func(req *http.Request) *http.Response {
			t.Fatalf("unexpected Graph request: %s", req.URL)
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), `unsupported well-known mail folder "Archive"`) {
		t.Fatalf("error = %v, want exact canonical-name rejection", err)
	}
	if calls != 0 {
		t.Fatalf("Graph handler calls = %d, want 0", calls)
	}
}

func TestMailFoldersOrdinaryPlainOutputKeepsExistingColumns(t *testing.T) {
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "folders", "list"},
		nil,
		func(req *http.Request) *http.Response {
			return graphJSONResponse(req, `{"value":[{"id":"folder-id","displayName":"Folder","totalItemCount":12,"unreadItemCount":3}]}`)
		},
	)
	if err != nil {
		t.Fatalf("mail folders list: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	firstLine := strings.SplitN(strings.TrimSpace(output), "\n", 2)[0]
	if strings.Contains(firstLine, "WELL-KNOWN") {
		t.Fatalf("ordinary list header changed: %q", firstLine)
	}
	for _, want := range []string{"ID", "NAME", "TOTAL", "UNREAD"} {
		if !strings.Contains(firstLine, want) {
			t.Errorf("ordinary list header %q missing %q", firstLine, want)
		}
	}
}

func TestMailMoveJSONReturnsStructuredReceipt(t *testing.T) {
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "move"},
		[]string{"--json", "source-id", "archive-id"},
		func(req *http.Request) *http.Response {
			if req.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", req.Method)
			}
			return graphJSONResponse(req, `{"id":"moved-id"}`)
		},
	)
	if err != nil {
		t.Fatalf("mail move --json: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}

	var envelope struct {
		Results []map[string]any `json:"results"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	want := map[string]any{
		"sourceId": "source-id",
		"id":       "moved-id",
		"status":   "succeeded",
		"code":     "move_succeeded",
	}
	if envelope.Count != 1 || len(envelope.Results) != 1 || !reflect.DeepEqual(envelope.Results[0], want) {
		t.Fatalf("move receipt = count %d, results %#v, want %#v", envelope.Count, envelope.Results, want)
	}
}

func TestMailGetJSONReturnsMessageObservations(t *testing.T) {
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "get"},
		[]string{"--json", "message-id"},
		func(req *http.Request) *http.Response {
			selected := req.URL.Query().Get("$select")
			for _, field := range []string{"parentFolderId", "changeKey", "flag", "isRead"} {
				if !containsCSVField(selected, field) {
					t.Errorf("$select = %q, want %s", selected, field)
				}
			}
			return graphJSONResponse(req, `{
				"id":"message-id",
				"parentFolderId":"folder-id",
				"changeKey":"version-1",
				"isRead":false,
				"flag":{"flagStatus":"flagged"}
			}`)
		},
	)
	if err != nil {
		t.Fatalf("mail get --json: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}

	var envelope struct {
		Results map[string]any `json:"results"`
		Count   int            `json:"count"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	want := map[string]any{
		"id":             "message-id",
		"parentFolderId": "folder-id",
		"changeKey":      "version-1",
		"isRead":         false,
		"flag":           map[string]any{"status": "flagged"},
	}
	for key, value := range want {
		if !reflect.DeepEqual(envelope.Results[key], value) {
			t.Errorf("message result[%q] = %#v, want %#v", key, envelope.Results[key], value)
		}
	}
	if envelope.Count != 1 {
		t.Fatalf("message result = count %d, result %#v", envelope.Count, envelope.Results)
	}
}

func containsCSVField(value, field string) bool {
	for _, candidate := range splitCSV(value) {
		if candidate == field {
			return true
		}
	}
	return false
}
