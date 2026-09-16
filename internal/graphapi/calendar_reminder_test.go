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
