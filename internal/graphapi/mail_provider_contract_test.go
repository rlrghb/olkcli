package graphapi

import (
	"context"
	"net/http"
	"path"
	"reflect"
	"strings"
	"testing"
)

func TestGetMessagePreservesAllRecipientClasses(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{
			"id":"message-id",
			"conversationId":"conversation-id",
			"toRecipients":[{"emailAddress":{"address":"to@example.com"}}],
			"ccRecipients":[{"emailAddress":{"address":"cc@example.com"}}],
			"bccRecipients":[{"emailAddress":{"address":"bcc@example.com"}}],
			"replyTo":[{"emailAddress":{"address":"reply@example.com"}}]
		}`)
	})

	message, err := client.GetMessage(
		context.Background(),
		"",
		"message-id",
		MessageBodyDefault,
	)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	for label, got := range map[string][]string{
		"to":      message.To,
		"cc":      message.Cc,
		"bcc":     message.Bcc,
		"replyTo": message.ReplyTo,
	} {
		want := []string{strings.ToLower(label) + "@example.com"}
		if label == "replyTo" {
			want = []string{"reply@example.com"}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", label, got, want)
		}
	}
}

func TestGetWellKnownMailFolderUsesCanonicalDelegatedRoute(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		if got, want := req.URL.Path, "/v1.0/users/shared@example.com/mailFolders/archive"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		return graphJSONResponse(req, `{"id":"folder-id","displayName":"Localized archive"}`)
	})

	folder, err := client.GetWellKnownMailFolder(context.Background(), "shared@example.com", "archive")
	if err != nil {
		t.Fatalf("GetWellKnownMailFolder() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
	if folder.ID != "folder-id" || folder.WellKnownName != "archive" {
		t.Fatalf("folder = %#v, want canonical archive mapping", folder)
	}
}

func TestGetWellKnownMailFolderRejectsUnsupportedNameBeforeRequest(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		return graphJSONResponse(req, `{}`)
	})

	folder, err := client.GetWellKnownMailFolder(context.Background(), "", "custom-folder")
	if err == nil || !strings.Contains(err.Error(), "unsupported well-known mail folder") {
		t.Fatalf("GetWellKnownMailFolder() error = %v, want unsupported-name rejection", err)
	}
	if folder != nil {
		t.Fatalf("folder = %#v, want nil", folder)
	}
	if requests != 0 {
		t.Fatalf("request count = %d, want 0", requests)
	}
}

func TestGetWellKnownMailFolderRejectsMissingProviderID(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{"displayName":"Archive"}`)
	})

	folder, err := client.GetWellKnownMailFolder(context.Background(), "", "archive")
	if err == nil || !strings.Contains(err.Error(), "missing folder ID") {
		t.Fatalf("GetWellKnownMailFolder() error = %v, want missing-ID rejection", err)
	}
	if folder != nil {
		t.Fatalf("folder = %#v, want nil", folder)
	}
}

func TestMoveMessageRequiresProviderDestinationID(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := path.Base(req.URL.Path); got != "move" {
			t.Errorf("request path = %q, want move action", req.URL.Path)
		}
		return graphJSONResponse(req, `{}`)
	})

	receipt, err := client.MoveMessage(context.Background(), "source-id", "folder-id")
	if err == nil || !strings.Contains(err.Error(), "graph returned no message response") {
		t.Fatalf("MoveMessage() error = %v, want missing-ID rejection", err)
	}
	if receipt != nil {
		t.Fatalf("receipt = %#v, want nil", receipt)
	}
}
