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

func TestCreateAllDayEventPreservesNamedTimeZone(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		var payload struct {
			Start struct {
				DateTime string `json:"dateTime"`
				TimeZone string `json:"timeZone"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				TimeZone string `json:"timeZone"`
			} `json:"end"`
			IsAllDay bool `json:"isAllDay"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode create payload: %v", err)
		}
		if !payload.IsAllDay || payload.Start.DateTime != "2026-11-01T00:00:00" || payload.End.DateTime != "2026-11-02T00:00:00" {
			t.Errorf("all-day boundaries = %#v, want local midnight across DST boundary", payload)
		}
		if payload.Start.TimeZone != "America/Los_Angeles" || payload.End.TimeZone != payload.Start.TimeZone {
			t.Errorf("boundary time zones = %q / %q, want America/Los_Angeles", payload.Start.TimeZone, payload.End.TimeZone)
		}
		return graphJSONResponse(req, `{"id":"event-id","isAllDay":true,"start":{"dateTime":"2026-11-01T00:00:00","timeZone":"America/Los_Angeles"},"end":{"dateTime":"2026-11-02T00:00:00","timeZone":"America/Los_Angeles"}}`)
	})

	start, err := time.Parse("2006-01-02", "2026-11-01")
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse("2006-01-02", "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	event, err := client.CreateEvent(context.Background(), &CreateEventOptions{
		Subject: "DST day", Start: start, End: end, IsAllDay: true, TimeZone: "America/Los_Angeles",
	})
	if err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}
	if event.StartTimeZone != "America/Los_Angeles" || event.EndTimeZone != "America/Los_Angeles" {
		t.Fatalf("returned time zones = %q / %q", event.StartTimeZone, event.EndTimeZone)
	}
}

func TestUpdateAllDayEventPreservesNamedTimeZone(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		var payload struct {
			Start struct {
				DateTime string `json:"dateTime"`
				TimeZone string `json:"timeZone"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				TimeZone string `json:"timeZone"`
			} `json:"end"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode update payload: %v", err)
		}
		if payload.Start.DateTime != "2026-03-08T00:00:00" || payload.End.DateTime != "2026-03-09T00:00:00" || payload.Start.TimeZone != "America/Los_Angeles" || payload.End.TimeZone != payload.Start.TimeZone {
			t.Errorf("update boundaries = %#v, want local midnight in America/Los_Angeles", payload)
		}
		return graphJSONResponse(req, `{"id":"event-id"}`)
	})
	start, _ := time.Parse("2006-01-02", "2026-03-08")
	end, _ := time.Parse("2006-01-02", "2026-03-09")
	allDay := true
	_, err := client.UpdateEvent(context.Background(), &UpdateEventOptions{
		EventID: "event-id", Start: &start, End: &end, AllDay: &allDay, TimeZone: "America/Los_Angeles",
	})
	if err != nil {
		t.Fatalf("UpdateEvent() error = %v", err)
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
