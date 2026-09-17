package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEventReminderOptionsRejectInvalidValues(t *testing.T) {
	disabled := false
	negative, zero := int32(-1), int32(0)
	for _, tc := range []struct {
		name      string
		on        *bool
		minutes   *int32
		wantError string
	}{
		{name: "negative", minutes: &negative, wantError: "nonnegative"},
		{name: "disabled with minutes", on: &disabled, minutes: &zero, wantError: "disabled reminder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				t.Fatal("invalid reminder must not send a Graph request")
				return nil
			})
			_, err := client.CreateEvent(context.Background(), &CreateEventOptions{
				Subject: "Event", Start: time.Now(), End: time.Now().Add(time.Hour),
				ReminderOn: tc.on, ReminderMinutes: tc.minutes,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("CreateEvent() error = %v, want %q", err, tc.wantError)
			}
			_, err = client.UpdateEvent(context.Background(), &UpdateEventOptions{
				EventID: "event-id", ReminderOn: tc.on, ReminderMinutes: tc.minutes,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("UpdateEvent() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestGetEventReminderJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want any
	}{
		{name: "absent", body: `{"id":"event-id"}`},
		{name: "zero", body: `{"id":"event-id","isReminderOn":true,"reminderMinutesBeforeStart":0}`, want: float64(0)},
		{name: "positive", body: `{"id":"event-id","isReminderOn":true,"reminderMinutesBeforeStart":30}`, want: float64(30)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				return graphJSONResponse(req, tc.body)
			})
			event, err := client.GetEvent(context.Background(), "", "event-id", BodyDefault)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			var output map[string]any
			if err := json.Unmarshal(data, &output); err != nil {
				t.Fatal(err)
			}
			got, present := output["reminderMinutesBeforeStart"]
			if got != tc.want || present != (tc.want != nil) {
				t.Fatalf("reminderMinutesBeforeStart = %v (present %v), want %v", got, present, tc.want)
			}
		})
	}
}

func TestDeltaCalendarViewSelectsReminderFields(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		selectFields := req.URL.Query().Get("$select")
		for _, field := range []string{"isReminderOn", "reminderMinutesBeforeStart"} {
			if !strings.Contains(selectFields, field) {
				t.Errorf("$select = %q, missing %q", selectFields, field)
			}
		}
		return graphJSONResponse(req, `{"value":[{"id":"event-id","isReminderOn":true,"reminderMinutesBeforeStart":0}],"@odata.deltaLink":"https://graph.microsoft.com/v1.0/me/calendarView/delta?$deltatoken=done"}`)
	})

	items, page, err := client.DeltaCalendarView(
		context.Background(), "", "",
		time.Date(2030, 1, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2030, 1, 16, 0, 0, 0, 0, time.UTC), 25,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ReminderMinutes == nil || *items[0].ReminderMinutes != 0 {
		t.Fatalf("delta reminder minutes = %#v, want one event with zero minutes", items)
	}
	if items[0].IsReminderOn == nil || !*items[0].IsReminderOn {
		t.Fatalf("delta reminder state = %#v, want enabled", items[0].IsReminderOn)
	}
	if !page.Complete {
		t.Fatal("delta page should be complete")
	}
}
