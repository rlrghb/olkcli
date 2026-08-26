package cmd

import (
	"net/http"
	"strings"
	"testing"
)

func TestMailListResolvesDelegatedFolderPath(t *testing.T) {
	requests := 0
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "list"},
		[]string{"--json", "--mailbox", "shared@example.com", "--folder", "Inbox/2026"},
		func(req *http.Request) *http.Response {
			requests++
			switch requests {
			case 1:
				if got := req.URL.Path; got != "/v1.0/users/shared@example.com/mailFolders" {
					t.Errorf("folder root request = %q", got)
				}
				return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":1}]}`)
			case 2:
				if got := req.URL.Path; got != "/v1.0/users/shared@example.com/mailFolders/inbox-id/childFolders" {
					t.Errorf("child-folder request = %q", got)
				}
				return graphJSONResponse(req, `{"value":[{"id":"year-id","displayName":"2026","childFolderCount":0}]}`)
			case 3:
				if got := req.URL.Path; got != "/v1.0/users/shared@example.com/mailFolders/year-id/messages" {
					t.Errorf("message-list request = %q, want resolved folder ID", got)
				}
				return graphMessageListResponse(req)
			default:
				t.Fatalf("unexpected Graph request: %s", req.URL)
				return nil
			}
		},
	)
	if err != nil {
		t.Fatalf("mail list: %v", err)
	}
	if calls != 3 {
		t.Fatalf("Graph requests = %d, want 3", calls)
	}
	if !strings.Contains(output, `"id": "message-id"`) {
		t.Errorf("mail list output omitted message: %s", output)
	}
}

func TestMailDeltaResolvesFolderPath(t *testing.T) {
	requests := 0
	_, calls, err := runMailCommand(
		t,
		[]string{"mail", "delta"},
		[]string{"--json", "--folder", "Inbox/2026"},
		func(req *http.Request) *http.Response {
			requests++
			switch requests {
			case 1:
				return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":1}]}`)
			case 2:
				return graphJSONResponse(req, `{"value":[{"id":"year-id","displayName":"2026","childFolderCount":0}]}`)
			case 3:
				if !strings.HasSuffix(req.URL.Path, "/mailFolders/year-id/messages/delta()") {
					t.Errorf("delta request path = %q, want resolved folder ID", req.URL.Path)
				}
				return graphJSONResponse(req, `{"value":[],"@odata.deltaLink":"https://graph.microsoft.com/v1.0/me/mailFolders/year-id/messages/delta?$deltatoken=done"}`)
			default:
				t.Fatalf("unexpected Graph request: %s", req.URL)
				return nil
			}
		},
	)
	if err != nil {
		t.Fatalf("mail delta: %v", err)
	}
	if calls != 3 {
		t.Fatalf("Graph requests = %d, want 3", calls)
	}
}

func TestMailMoveResolvesFolderPathToDestinationID(t *testing.T) {
	requests := 0
	_, calls, err := runMailCommand(
		t,
		[]string{"mail", "move"},
		[]string{"message-id", "Inbox/2026"},
		func(req *http.Request) *http.Response {
			requests++
			switch requests {
			case 1:
				return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":1}]}`)
			case 2:
				return graphJSONResponse(req, `{"value":[{"id":"year-id","displayName":"2026","childFolderCount":0}]}`)
			case 3:
				if !strings.HasSuffix(req.URL.Path, "/messages/message-id/move") {
					t.Errorf("move request path = %q", req.URL.Path)
				}
				var payload struct {
					DestinationID string `json:"destinationId"`
				}
				if err := decodeGraphJSON(req.Body, &payload); err != nil {
					t.Fatalf("decode move request: %v", err)
				}
				if payload.DestinationID != "year-id" {
					t.Errorf("destination ID = %q, want resolved year-id", payload.DestinationID)
				}
				return graphJSONResponse(req, `{"id":"moved-id"}`)
			default:
				t.Fatalf("unexpected Graph request: %s", req.URL)
				return nil
			}
		},
	)
	if err != nil {
		t.Fatalf("mail move: %v", err)
	}
	if calls != 3 {
		t.Fatalf("Graph requests = %d, want 3", calls)
	}
}

func TestMailMoveDryRunDoesNotResolveFolderPath(t *testing.T) {
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "move"},
		[]string{"--dry-run", "message-id", "Inbox/2026"},
		func(req *http.Request) *http.Response {
			t.Fatalf("unexpected Graph request during dry run: %s", req.URL)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("mail move --dry-run: %v", err)
	}
	if calls != 0 {
		t.Fatalf("Graph requests = %d, want 0", calls)
	}
	if got, want := output, "Would move message message-id to folder Inbox/2026\n"; got != want {
		t.Fatalf("dry-run output = %q, want %q", got, want)
	}
}
