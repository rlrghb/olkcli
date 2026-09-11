package cmd

import (
	"net/http"
	"strings"
	"testing"
)

func graphNoContentResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{},
		Body:       http.NoBody,
		Request:    req,
	}
}

func TestMailDeleteAddressesTheRequestedMailbox(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		wantPath string
		wantOut  string
	}{
		{
			name:     "own mailbox",
			args:     []string{"--force", "message-id"},
			wantPath: "/v1.0/me/messages/message-id",
			wantOut:  "Message deleted.\n",
		},
		{
			name:     "delegated mailbox",
			args:     []string{"--force", "--mailbox", "shared@example.com", "message-id"},
			wantPath: "/v1.0/users/shared@example.com/messages/message-id",
			wantOut:  "Message deleted from shared@example.com.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotMethod string
			output, calls, err := runMailCommand(t, []string{"mail", "delete"}, tc.args,
				func(req *http.Request) *http.Response {
					gotPath, gotMethod = req.URL.Path, req.Method
					return graphNoContentResponse(req)
				})
			if err != nil {
				t.Fatalf("mail delete: %v", err)
			}
			if calls != 1 || gotMethod != http.MethodDelete || gotPath != tc.wantPath {
				t.Fatalf("calls=%d method=%s path=%q, want one DELETE of %q", calls, gotMethod, gotPath, tc.wantPath)
			}
			if output != tc.wantOut {
				t.Fatalf("output = %q, want %q", output, tc.wantOut)
			}
		})
	}
}

func TestMailDeleteRefusesWithoutForce(t *testing.T) {
	_, calls, err := runMailCommand(t, []string{"mail", "delete"},
		[]string{"--mailbox", "shared@example.com", "message-id"},
		func(req *http.Request) *http.Response {
			t.Fatalf("unexpected request: %s", req.URL)
			return nil
		})
	if err == nil || !strings.Contains(err.Error(), "--force") || calls != 0 {
		t.Fatalf("error=%v calls=%d, want a --force refusal before any request", err, calls)
	}
}

func TestMailDeleteInvalidMailboxAndDelegatedDryRun(t *testing.T) {
	for _, tc := range []struct {
		mailbox   string
		wantError bool
	}{{"not-an-address", true}, {"shared@example.com", false}} {
		t.Run(tc.mailbox, func(t *testing.T) {
			output, calls, err := runMailCommand(t, []string{"mail", "delete"},
				[]string{"message-id", "--force", "--mailbox", tc.mailbox, "--dry-run"},
				func(req *http.Request) *http.Response {
					t.Fatalf("unexpected request: %s", req.URL)
					return nil
				})
			if (err != nil) != tc.wantError || calls != 0 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if !tc.wantError && output != "Would delete message message-id from shared@example.com\n" {
				t.Fatalf("preview = %q, want it to name the target mailbox", output)
			}
		})
	}
}
