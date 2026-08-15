package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCalendarCreateRoutesAndReturnsJSON(t *testing.T) {
	tests := []struct {
		name         string
		calendarID   string
		expectedPath string
	}{
		{name: "default calendar", expectedPath: "/v1.0/me/events"},
		{name: "selected calendar", calendarID: "calendar-id", expectedPath: "/v1.0/me/calendars/calendar-id/events"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{
				"--json",
				"--subject", "Proof event",
				"--start", "2026-08-14T19:00:00Z",
				"--end", "2026-08-14T19:15:00Z",
			}
			if tc.calendarID != "" {
				args = append(args, "--calendar", tc.calendarID)
			}

			output, calls, err := runMailCommand(
				t,
				[]string{"calendar", "create"},
				args,
				func(req *http.Request) *http.Response {
					if req.Method != http.MethodPost {
						t.Errorf("request method = %s, want POST", req.Method)
					}
					if req.URL.Path != tc.expectedPath {
						t.Errorf("request path = %q, want %q", req.URL.Path, tc.expectedPath)
					}
					return graphJSONResponse(req, `{
						"id":"event-id",
						"subject":"Proof event",
						"start":{"dateTime":"2026-08-14T19:00:00","timeZone":"UTC"},
						"end":{"dateTime":"2026-08-14T19:15:00","timeZone":"UTC"}
					}`)
				},
			)
			if err != nil {
				t.Fatalf("calendar create: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph handler calls = %d, want 1", calls)
			}

			var envelope struct {
				Results struct {
					ID      string `json:"id"`
					Subject string `json:"subject"`
				} `json:"results"`
				Count int `json:"count"`
			}
			if err := json.Unmarshal([]byte(output), &envelope); err != nil {
				t.Fatalf("decode JSON output: %v\n%s", err, output)
			}
			if envelope.Count != 1 || envelope.Results.ID != "event-id" || envelope.Results.Subject != "Proof event" {
				t.Fatalf("created event receipt = %#v, want event-id and Proof event", envelope)
			}
		})
	}
}

func TestCalendarCreateRejectsInvalidCalendarIDWithoutRequest(t *testing.T) {
	_, calls, err := runMailCommand(
		t,
		[]string{"calendar", "create"},
		[]string{
			"--calendar", "not a calendar id",
			"--subject", "Proof event",
			"--start", "2026-08-14T19:00:00Z",
			"--end", "2026-08-14T19:15:00Z",
		},
		func(req *http.Request) *http.Response {
			t.Fatalf("unexpected Graph request: %s", req.URL)
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "calendar ID contains invalid characters") {
		t.Fatalf("error = %v, want invalid calendar ID rejection", err)
	}
	if calls != 0 {
		t.Fatalf("Graph handler calls = %d, want 0", calls)
	}
}
