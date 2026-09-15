package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCalendarAttachmentLifecycleUsesEventRoute(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		switch req.Method {
		case http.MethodGet:
			if req.URL.Path == meBuilderPath+"/events/event-id/attachments" {
				return graphJSONResponse(req, `{"value":[{"id":"attachment-id","name":"notes.txt","contentType":"text/plain","size":4}]}`)
			}
			if req.URL.Path == meBuilderPath+"/events/event-id/attachments/attachment-id" {
				return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id","name":"notes.txt","contentType":"text/plain","size":4,"contentBytes":"dGVzdA=="}`)
			}
		case http.MethodPost:
			if req.URL.Path != meBuilderPath+"/events/event-id/attachments" {
				t.Fatalf("POST path = %q", req.URL.Path)
			}
			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode upload payload: %v", err)
			}
			if payload["name"] != "notes.txt" || payload["contentType"] != "text/plain" {
				t.Errorf("upload payload = %#v", payload)
			}
			return graphJSONResponse(req, `{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment-id","name":"notes.txt","contentType":"text/plain","size":4}`)
		case http.MethodDelete:
			if req.URL.Path != meBuilderPath+"/events/event-id/attachments/attachment-id" {
				t.Fatalf("DELETE path = %q", req.URL.Path)
			}
			return graphEmptyResponse(req)
		}
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		return nil
	})

	attachments, err := client.ListCalendarAttachments(context.Background(), "event-id")
	if err != nil || len(attachments) != 1 || attachments[0].Name != "notes.txt" {
		t.Fatalf("ListCalendarAttachments() = %#v, error = %v", attachments, err)
	}
	created, err := client.UploadCalendarAttachment(context.Background(), "event-id", "notes.txt", "text/plain", []byte("test"))
	if err != nil || created.ID != "attachment-id" {
		t.Fatalf("UploadCalendarAttachment() = %#v, error = %v", created, err)
	}
	downloaded, content, err := client.DownloadCalendarAttachment(context.Background(), "event-id", "attachment-id")
	if err != nil || downloaded.Name != "notes.txt" || string(content) != "test" {
		t.Fatalf("DownloadCalendarAttachment() = %#v, %q, error = %v", downloaded, content, err)
	}
	if err := client.DeleteCalendarAttachment(context.Background(), "event-id", "attachment-id"); err != nil {
		t.Fatalf("DeleteCalendarAttachment() error = %v", err)
	}
}

func TestUploadCalendarAttachmentRejectsThreeMBFile(t *testing.T) {
	client := &Client{noWrite: false}
	_, err := client.UploadCalendarAttachment(context.Background(), "event-id", "large.bin", "application/octet-stream", make([]byte, MaxCalendarAttachmentBytes))
	if err == nil || !strings.Contains(err.Error(), "under 3 MB") {
		t.Fatalf("error = %v, want size rejection", err)
	}
}
