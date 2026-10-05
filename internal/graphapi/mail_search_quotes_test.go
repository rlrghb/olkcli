package graphapi

import (
	"context"
	"net/http"
	"testing"
)

func TestSearchMessagesEscapesQuotes(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantSearch string
	}{
		{
			name:       "exact phrase with quotes",
			query:      `subject:"monthly report"`,
			wantSearch: `"subject:\"monthly report\""`,
		},
		{
			name:       "quotes and backslashes",
			query:      `body:"path\to\file"`,
			wantSearch: `"body:\"path\\to\\file\""`,
		},
		{
			name:       "simple query without quotes",
			query:      `urgent`,
			wantSearch: `"urgent"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotSearch string
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				gotSearch = req.URL.Query().Get("$search")
				return graphJSONResponse(req, `{"value":[]}`)
			})
			_, err := client.SearchMessages(context.Background(), "", tc.query, 10)
			if err != nil {
				t.Fatalf("SearchMessages: %v", err)
			}
			if gotSearch != tc.wantSearch {
				t.Errorf("$search = %q, want %q", gotSearch, tc.wantSearch)
			}
		})
	}
}
