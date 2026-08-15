package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestListEventsRequestsProviderTextBodies(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("Prefer = %q, want provider text preference", got)
		}
		if !strings.Contains(req.URL.Query().Get("$select"), "body") {
			t.Errorf("$select = %q, want body", req.URL.Query().Get("$select"))
		}
		response := graphJSONResponse(req, `{"value":[{
			"id":"event-one",
			"subject":"Event",
			"start":{"dateTime":"2026-08-14T19:00:00","timeZone":"UTC"},
			"end":{"dateTime":"2026-08-14T20:00:00","timeZone":"UTC"},
			"body":{"contentType":"text","content":"Complete event body"}
		}]}`)
		response.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return response
	})

	events, err := client.ListEvents(
		context.Background(), "", time.Now(), time.Now().Add(time.Hour), "", 25, BodyText,
	)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Body != "Complete event body" || events[0].BodyType != "text" {
		t.Fatalf("ListEvents() events = %#v, want complete provider text body", events)
	}
}

func TestGetEventRequestsProviderTextBody(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("Prefer = %q, want provider text preference", got)
		}
		response := graphJSONResponse(req, `{
			"id":"event-one",
			"body":{"contentType":"text","content":"Complete event body"}
		}`)
		response.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return response
	})

	event, err := client.GetEvent(context.Background(), "", "event-one", BodyText)
	if err != nil {
		t.Fatalf("GetEvent() error = %v", err)
	}
	if event.Body != "Complete event body" || event.BodyType != "text" {
		t.Fatalf("GetEvent() body = %q type = %q, want provider text", event.Body, event.BodyType)
	}
}

func TestListEventsRejectsMissingProviderAcknowledgement(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{"value":[]}`)
	})

	events, err := client.ListEvents(
		context.Background(), "", time.Now(), time.Now().Add(time.Hour), "", 25, BodyText,
	)
	if err == nil || !strings.Contains(err.Error(), "Preference-Applied") {
		t.Fatalf("ListEvents() error = %v, want missing acknowledgement rejection", err)
	}
	if events != nil {
		t.Fatalf("ListEvents() events = %#v, want nil", events)
	}
}
