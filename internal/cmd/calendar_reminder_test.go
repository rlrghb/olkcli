package cmd

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestCalendarReminderFlags(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		t.Run(command, func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				args        []string
				wantOn      any
				wantMinutes any
				wantError   string
			}{
				{name: "omitted"},
				{name: "disabled", args: []string{"--no-reminder"}, wantOn: false},
				{name: "enabled", args: []string{"--reminder-minutes", "30"}, wantOn: true, wantMinutes: float64(30)},
				{name: "at start", args: []string{"--reminder-minutes", "0"}, wantOn: true, wantMinutes: float64(0)},
				{name: "negative", args: []string{"--reminder-minutes=-1"}, wantError: "nonnegative"},
				{name: "conflicting", args: []string{"--no-reminder", "--reminder-minutes", "0"}, wantError: "mutually exclusive"},
				{name: "overflow", args: []string{"--reminder-minutes", "2147483648"}, wantError: "expected a valid 32 bit int"},
				{name: "no write", args: []string{"--no-write", "--reminder-minutes", "15"}, wantError: "writes are disabled"},
				{name: "no send", args: []string{"--no-send", "--reminder-minutes", "15"}, wantOn: true, wantMinutes: float64(15)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					_, calls, err := runCalendarReminderCommand(t, command, tc.args, func(req *http.Request) *http.Response {
						var payload map[string]any
						if err := decodeGraphJSON(req.Body, &payload); err != nil {
							t.Fatalf("decode event payload: %v", err)
						}
						for field, want := range map[string]any{"isReminderOn": tc.wantOn, "reminderMinutesBeforeStart": tc.wantMinutes} {
							got, present := payload[field]
							if got != want || present != (want != nil) {
								t.Errorf("%s = %v (present %v), want %v", field, got, present, want)
							}
						}
						return graphJSONResponse(req, `{"id":"event-id","subject":"Event"}`)
					})
					if tc.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), tc.wantError) || calls != 0 {
							t.Fatalf("error = %v, calls = %d; want %q and no requests", err, calls, tc.wantError)
						}
					} else if err != nil || calls != 1 {
						t.Fatalf("error = %v, calls = %d; want one successful request", err, calls)
					}
				})
			}
		})
	}
}

func TestCalendarReminderCreateDryRunAndSendGuard(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantError  string
		wantOutput string
	}{
		{name: "dry run", args: []string{"--dry-run", "--reminder-minutes", "0"}, wantOutput: "Reminder: 0 minutes before start"},
		{name: "attendees", args: []string{"--no-send", "--attendees", "guest@example.com", "--reminder-minutes", "15"}, wantError: "sending is disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, calls, err := runCalendarReminderCommand(t, "create", tc.args, func(req *http.Request) *http.Response {
				return graphJSONResponse(req, `{}`)
			})
			if calls != 0 {
				t.Fatalf("requests = %d, want none", calls)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil || !strings.Contains(output, tc.wantOutput) {
				t.Fatalf("output = %q, error = %v; want %q", output, err, tc.wantOutput)
			}
		})
	}
}

func runCalendarReminderCommand(t *testing.T, command string, args []string, responder func(*http.Request) *http.Response) (output string, calls int, err error) {
	t.Helper()
	client := testMailListClient(t, func(req *http.Request) *http.Response {
		calls++
		return responder(req)
	})
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		return "", calls, err
	}
	argv := []string{"calendar", command}
	if command == "create" {
		argv = append(argv, "--subject", "Event", "--start", "2030-01-15T09:00:00Z", "--end", "2030-01-15T10:00:00Z")
	} else {
		argv = append(argv, "event-id", "--subject", "Updated event")
	}
	kctx, err := parser.Parse(append(argv, args...))
	if err != nil {
		return "", calls, err
	}
	client.SetGuards(cli.NoWrite, cli.NoSend)
	output, _, err = captureStd(func() error {
		return kctx.Run(&RunContext{Ctx: context.Background(), Flags: &cli.RootFlags, client: client})
	})
	return output, calls, err
}
