package cmd

import (
	"net/http"
	"strings"
	"testing"
)

// Model Graph's documented filter/orderby contract. This is a strict contract
// fixture, not a recording of live service behavior: Outlook.com has accepted
// the unread-only cases without these constraints. Sender cases are reported
// to fail live; the unread cases protect conformance to the documented contract.
func TestMailListFilteredSortMeetsGraphContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unread", []string{"--unread"}},
		{"sender", []string{"--from", "recruiter@example.com"}},
		{"unread after", []string{"--unread", "--after", "2026-09-01"}},
		{"sender before", []string{"--from", "recruiter@example.com", "--before", "2026-09-27"}},
		{"combined oldest", []string{"--unread", "--from", "recruiter@example.com", "--after", "2026-09-01", "--before", "2026-09-27", "--order", "oldest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, calls, err := runMailCommand(t, []string{"mail", "list"}, append([]string{"--json"}, tc.args...), func(req *http.Request) *http.Response {
				query := req.URL.Query()
				filter := query.Get("$filter")
				if query.Get("$orderby") == "" {
					t.Error("filtered list lost the requested chronological ordering")
				}
				validOrder := strings.HasPrefix(filter, "receivedDateTime ")
				for _, property := range []string{"isRead", "from/emailAddress/address"} {
					if index := strings.Index(filter, property); index >= 0 && index < strings.LastIndex(filter, "receivedDateTime") {
						validOrder = false
					}
				}
				if !validOrder {
					response := graphJSONResponse(req, `{"error":{"code":"InefficientFilter","message":"The restriction or sort order is too complex for this operation."}}`)
					response.StatusCode = http.StatusBadRequest
					return response
				}
				return graphMessageListResponse(req)
			})
			if err != nil {
				t.Fatalf("filtered mail list: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph calls = %d, want 1", calls)
			}
		})
	}
}
