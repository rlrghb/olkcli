package graphapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

func TestCalendarOriginalTimeZones(t *testing.T) {
	const event = `{"id":"event-id","originalStartTimeZone":"Pacific Standard Time","originalEndTimeZone":"tzone://Microsoft/Custom"}`
	start := time.Date(2030, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, mode := range []string{"get", "view", "named view", "delta"} {
		t.Run(mode, func(t *testing.T) {
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				if mode == "view" || mode == "named view" {
					selected := strings.Split(req.URL.Query().Get("$select"), ",")
					for _, want := range []string{"originalStartTimeZone", "originalEndTimeZone"} {
						found := false
						for _, field := range selected {
							if field == want {
								found = true
							}
						}
						if !found {
							t.Errorf("missing %s in $select: %v", want, selected)
						}
					}
				}
				if mode == "get" {
					return graphJSONResponse(req, event)
				}
				return graphJSONResponse(req, `{"value":[`+event+`]}`)
			})
			var got CalendarEvent
			switch mode {
			case "get":
				e, err := client.GetEvent(context.Background(), "", "event-id", BodyDefault)
				if err != nil {
					t.Fatal(err)
				}
				got = *e
			case "delta":
				events, _, err := client.DeltaCalendarView(context.Background(), "", "", start, start.Add(24*time.Hour), 25)
				if err != nil {
					t.Fatal(err)
				}
				if len(events) != 1 {
					t.Fatalf("events: %v", events)
				}
				got = events[0].CalendarEvent
			default:
				calendar := ""
				if mode == "named view" {
					calendar = "calendar-id"
				}
				events, err := client.ListEvents(context.Background(), "", start, start.Add(24*time.Hour), calendar, 25, BodyDefault)
				if err != nil {
					t.Fatal(err)
				}
				if len(events) != 1 {
					t.Fatalf("events: %v", events)
				}
				got = events[0]
			}
			if got.OriginalStartTimeZone != "Pacific Standard Time" || got.OriginalEndTimeZone != "tzone://Microsoft/Custom" {
				t.Fatalf("time zones: %#v", got)
			}
			var buf bytes.Buffer
			p := outfmt.Printer{Writer: &buf, WrapUntrusted: true, ResultsOnly: true}
			if err := p.PrintJSON(got, 1, ""); err != nil {
				t.Fatal(err)
			}
			var wrapped map[string]any
			if err := json.Unmarshal(buf.Bytes(), &wrapped); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"originalStartTimeZone", "originalEndTimeZone"} {
				value, ok := wrapped[key].(string)
				if !ok || !strings.HasPrefix(value, "[UNTRUSTED:") || !strings.Contains(value, "[/UNTRUSTED:") {
					t.Errorf("%s not wrapped: %v", key, wrapped[key])
				}
			}
		})
	}
}

func TestCalendarOriginalTimeZonesOmittedWhenMissing(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{"id":"event-id","originalStartTimeZone":null}`)
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
	for _, key := range []string{"originalStartTimeZone", "originalEndTimeZone"} {
		if _, ok := output[key]; ok {
			t.Errorf("unexpected %s: %s", key, data)
		}
	}
}
