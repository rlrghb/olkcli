package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreateEventIncludesBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		html     bool
		wantType string
		wantBody string
	}{
		{name: "text", wantType: "text", wantBody: "Agenda"},
		{name: "html", html: true, wantType: "html", wantBody: "<p>Agenda</p>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				var payload struct {
					Body struct {
						ContentType string `json:"contentType"`
						Content     string `json:"content"`
					} `json:"body"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode create payload: %v", err)
				}
				if payload.Body.ContentType != tc.wantType || payload.Body.Content != tc.wantBody {
					t.Errorf("body payload = %#v, want type=%q content=%q", payload.Body, tc.wantType, tc.wantBody)
				}
				return graphJSONResponse(req, `{"id":"event-id","subject":"Event"}`)
			})

			_, err := client.CreateEvent(context.Background(), &CreateEventOptions{
				Subject: "Event", Start: time.Now(), End: time.Now().Add(time.Hour),
				Body: &EventBodyInput{Content: tc.wantBody, HTML: tc.html},
			})
			if err != nil {
				t.Fatalf("CreateEvent() error = %v", err)
			}
		})
	}
}

func TestUpdateEventPreservesOnlineMeetingBody(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if req.Method == http.MethodGet {
			resp := graphJSONResponse(req, `{
				"id":"event-id",
				"isOnlineMeeting":true,
				"body":{"contentType":"html","content":"<html><body><p>Old notes</p><div class=\"me-email-text\"><a>Join Microsoft Teams Meeting</a></div></body></html>"}
			}`)
			resp.Header.Set("Preference-Applied", `outlook.body-content-type="html"`)
			return resp
		}
		var payload struct {
			Body struct {
				ContentType string `json:"contentType"`
				Content     string `json:"content"`
			} `json:"body"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode update payload: %v", err)
		}
		if payload.Body.ContentType != "html" {
			t.Errorf("body content type = %q, want html", payload.Body.ContentType)
		}
		if !strings.Contains(payload.Body.Content, "New notes") || !strings.Contains(payload.Body.Content, "me-email-text") {
			t.Errorf("updated body = %q, want new notes and preserved meeting section", payload.Body.Content)
		}
		return graphJSONResponse(req, `{"id":"event-id","subject":"Event"}`)
	})

	_, err := client.UpdateEvent(context.Background(), &UpdateEventOptions{
		EventID: "event-id", Body: &EventBodyInput{Content: "New notes"},
	})
	if err != nil {
		t.Fatalf("UpdateEvent() error = %v", err)
	}
}

func TestPreserveOnlineMeetingBodyRejectsUnknownProviderBody(t *testing.T) {
	_, err := preserveOnlineMeetingBody("<html><body><p>Meeting</p></body></html>", "Notes", false)
	if err == nil || !strings.Contains(err.Error(), "could not identify") {
		t.Fatalf("error = %v, want unknown provider meeting section", err)
	}
}
