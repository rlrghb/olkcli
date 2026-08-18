package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestListEventsPreservesProviderMetadataAndStructuredCalendarState(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		selectFields := req.URL.Query().Get("$select")
		for _, field := range []string{"iCalUId", "changeKey", "type", "seriesMasterId", "originalStart", "isCancelled", "responseStatus", "body"} {
			if !strings.Contains(selectFields, field) {
				t.Errorf("$select = %q, missing %q", selectFields, field)
			}
		}
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("Prefer = %q, want text body preference", got)
		}
		resp := graphJSONResponse(req, `{"value":[{
			"id":"event-1","subject":"Planning","iCalUId":"ical-1","changeKey":"key-2",
			"type":"exception","seriesMasterId":"master-1","originalStart":"2026-08-18T09:00:00Z",
			"isCancelled":false,"body":{"contentType":"text","content":"Agenda"},
			"attendees":[{"emailAddress":{"name":"Alex","address":"alex@example.com"},"type":"optional","status":{"response":"accepted","time":"2026-08-17T12:00:00Z"}}],
			"responseStatus":{"response":"tentativelyAccepted","time":"2026-08-17T12:00:00Z"},
			"recurrence":{"pattern":{"type":"weekly","interval":1,"daysOfWeek":["monday"]},"range":{"type":"endDate","startDate":"2026-08-18","endDate":"2026-09-01"}}
		}]}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})

	events, err := client.ListEvents(context.Background(), "", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC), "", 25, BodyText)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	event := events[0]
	if event.ICalUID != "ical-1" || event.ChangeKey != "key-2" || event.EventType != "exception" || event.SeriesMasterID != "master-1" {
		t.Fatalf("provider metadata = %#v", event)
	}
	if event.Body != "Agenda" || event.BodyType != "text" || len(event.AttendeeDetails) != 1 || event.ResponseStatus == nil || event.RecurrenceDetails == nil {
		t.Fatalf("structured event state = %#v", event)
	}
	if event.AttendeeDetails[0].Response != "accepted" || event.RecurrenceDetails.PatternType != "weekly" || event.RecurrenceDetails.RangeType != "endDate" {
		t.Fatalf("structured event details = %#v", event)
	}
}

func TestListMessagesAndContactsPreserveProviderMetadata(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		if strings.HasSuffix(req.URL.Path, "/messages") {
			selectFields := req.URL.Query().Get("$select")
			for _, field := range []string{"internetMessageId", "createdDateTime", "lastModifiedDateTime"} {
				if !strings.Contains(selectFields, field) {
					t.Errorf("message $select = %q, missing %q", selectFields, field)
				}
			}
			return graphJSONResponse(req, `{"value":[{"id":"message-1","internetMessageId":"<message@example.com>","createdDateTime":"2026-08-17T10:00:00Z","lastModifiedDateTime":"2026-08-17T10:01:00Z"}]}`)
		}
		if strings.HasSuffix(req.URL.Path, "/contacts") {
			return graphJSONResponse(req, `{"value":[{"id":"contact-1","displayName":"Alex","createdDateTime":"2026-08-17T10:00:00Z","lastModifiedDateTime":"2026-08-17T10:01:00Z","changeKey":"contact-key"}]}`)
		}
		t.Fatalf("unexpected request path %q", req.URL.Path)
		return nil
	})

	messages, err := client.ListMessages(context.Background(), "", &ListMessagesOptions{Top: 1})
	if err != nil || len(messages) != 1 || messages[0].InternetMessageID != "<message@example.com>" || messages[0].CreatedAt == "" || messages[0].ModifiedAt == "" {
		t.Fatalf("messages = %#v, error = %v", messages, err)
	}
	contacts, err := client.ListContacts(context.Background(), "", 1, 0, "")
	if err != nil || len(contacts) != 1 || contacts[0].ChangeKey != "contact-key" || contacts[0].CreatedAt == "" || contacts[0].ModifiedAt == "" {
		t.Fatalf("contacts = %#v, error = %v", contacts, err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}
