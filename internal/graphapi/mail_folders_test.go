package graphapi

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestListMailFoldersTraversesVisibleChildren(t *testing.T) {
	requests := make([]string, 0, 3)
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests = append(requests, req.URL.Path)
		if got := req.URL.Query().Get("$select"); !strings.Contains(got, "childFolderCount") || !strings.Contains(got, "parentFolderId") {
			t.Errorf("$select = %q, want traversal fields", got)
		}
		switch req.URL.Path {
		case "/v1.0/users/shared@example.com/mailFolders":
			return graphJSONResponse(req, `{"value":[
				{"id":"inbox-id","displayName":"Inbox","childFolderCount":1},
				{"id":"archive-id","displayName":"Archive","childFolderCount":0}
			]}`)
		case "/v1.0/users/shared@example.com/mailFolders/inbox-id/childFolders":
			return graphJSONResponse(req, `{"value":[
				{"id":"year-id","displayName":"2026","parentFolderId":"inbox-id","childFolderCount":1}
			]}`)
		case "/v1.0/users/shared@example.com/mailFolders/year-id/childFolders":
			return graphJSONResponse(req, `{"value":[
				{"id":"receipts-id","displayName":"Receipts","parentFolderId":"year-id","childFolderCount":0}
			]}`)
		default:
			t.Fatalf("unexpected Graph request: %s", req.URL)
			return nil
		}
	})

	folders, err := client.ListMailFolders(context.Background(), "shared@example.com")
	if err != nil {
		t.Fatalf("ListMailFolders() error = %v", err)
	}
	if got, want := requests, []string{
		"/v1.0/users/shared@example.com/mailFolders",
		"/v1.0/users/shared@example.com/mailFolders/inbox-id/childFolders",
		"/v1.0/users/shared@example.com/mailFolders/year-id/childFolders",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("request paths = %v, want %v", got, want)
	}
	if got, want := folderIDs(folders), []string{"inbox-id", "archive-id", "year-id", "receipts-id"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folder IDs = %v, want breadth-first traversal %v", got, want)
	}
	if folders[2].ParentFolderID != "inbox-id" || folders[2].ChildFolderCount != 1 {
		t.Errorf("converted child folder = %#v, want parent and child count", folders[2])
	}
}

func TestListMailFoldersFollowsRootContinuation(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		if requests == 1 {
			next := "https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders?$skiptoken=next"
			return graphJSONResponse(req, `{"value":[{"id":"one","displayName":"One","childFolderCount":0}],"@odata.nextLink":"`+next+`"}`)
		}
		if got := req.URL.Query().Get("$skiptoken"); got != "next" {
			t.Errorf("continuation skip token = %q, want next", got)
		}
		return graphJSONResponse(req, `{"value":[{"id":"two","displayName":"Two","childFolderCount":0}]}`)
	})

	folders, err := client.ListMailFolders(context.Background(), "shared@example.com")
	if err != nil {
		t.Fatalf("ListMailFolders() error = %v", err)
	}
	if got, want := folderIDs(folders), []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folder IDs = %v, want %v", got, want)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestResolveMailFolderPathWalksDelegatedMailboxAndChildPages(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		switch requests {
		case 1:
			if got := req.URL.Path; got != "/v1.0/users/shared@example.com/mailFolders" {
				t.Errorf("root path = %q", got)
			}
			return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":2}]}`)
		case 2:
			next := "https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders/inbox-id/childFolders?$skiptoken=next"
			return graphJSONResponse(req, `{"value":[{"id":"old-id","displayName":"2025","childFolderCount":0}],"@odata.nextLink":"`+next+`"}`)
		case 3:
			if got := req.URL.Query().Get("$skiptoken"); got != "next" {
				t.Errorf("child continuation skip token = %q, want next", got)
			}
			return graphJSONResponse(req, `{"value":[{"id":"year-id","displayName":"2026","childFolderCount":0}]}`)
		default:
			t.Fatalf("unexpected Graph request: %s", req.URL)
			return nil
		}
	})

	id, err := client.ResolveMailFolderPath(context.Background(), "shared@example.com", "inbox/2026")
	if err != nil {
		t.Fatalf("ResolveMailFolderPath() error = %v", err)
	}
	if id != "year-id" {
		t.Fatalf("resolved ID = %q, want year-id", id)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestResolveMailFolderPathPreservesSlashBearingGraphID(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":0}]}`)
	})

	const graphID = "AAMkAGVm/AAA="
	got, err := client.ResolveMailFolderPath(context.Background(), "", graphID)
	if err != nil {
		t.Fatalf("ResolveMailFolderPath() error = %v", err)
	}
	if got != graphID {
		t.Fatalf("resolved reference = %q, want original Graph ID %q", got, graphID)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want root lookup only", requests)
	}
}

func TestResolveMailFolderPathRejectsMissingAndAmbiguousComponents(t *testing.T) {
	tests := []struct {
		name     string
		children string
		want     string
	}{
		{
			name:     "missing child",
			children: `{"value":[]}`,
			want:     `component "2026" not found`,
		},
		{
			name: "ambiguous child",
			children: `{"value":[
				{"id":"one","displayName":"2026","childFolderCount":0},
				{"id":"two","displayName":"2026","childFolderCount":0}
			]}`,
			want: `folder name "2026" is ambiguous`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				requests++
				if requests == 1 {
					return graphJSONResponse(req, `{"value":[{"id":"inbox-id","displayName":"Inbox","childFolderCount":1}]}`)
				}
				return graphJSONResponse(req, tc.children)
			})
			id, err := client.ResolveMailFolderPath(context.Background(), "", "Inbox/2026")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ResolveMailFolderPath() = (%q, %v), want %q error", id, err, tc.want)
			}
		})
	}
}

func TestValidateGraphContinuationAllowsMailFolderCollections(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		scope graphContinuationScope
	}{
		{
			name:  "root folders",
			url:   "https://graph.microsoft.com/v1.0/me/mailFolders?$skiptoken=next",
			scope: continuationScope("graph.microsoft.com", "/v1.0/me/mailFolders"),
		},
		{
			name: "delegated child folders",
			url:  "https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders/" + url.PathEscape("AAMk/AAA=") + "/childFolders?$skiptoken=next",
			scope: continuationScope(
				"graph.microsoft.com",
				"/v1.0/users/shared@example.com/mailFolders/"+url.PathEscape("AAMk/AAA=")+"/childFolders",
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateGraphContinuation(tc.url, tc.scope); err != nil {
				t.Fatalf("validateGraphContinuation() error = %v", err)
			}
		})
	}
}

func folderIDs(folders []MailFolder) []string {
	ids := make([]string, len(folders))
	for i := range folders {
		ids[i] = folders[i].ID
	}
	return ids
}
