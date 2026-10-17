package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestMailDeltaSelectsSenderReceivedAndReadState(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		sel := req.URL.Query().Get("$select")
		for _, field := range []string{"from", "receivedDateTime", "isRead"} {
			if !strings.Contains(","+sel+",", ","+field+",") {
				t.Errorf("$select = %q, want it to include %q", sel, field)
			}
		}
		return graphJSONResponse(req, `{"value":[{"id":"m1","subject":"hi","isRead":true,"receivedDateTime":"2026-10-16T05:57:34Z","from":{"emailAddress":{"address":"sender@example.com"}}}],"@odata.deltaLink":"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages/delta?$deltatoken=x"}`)
	})
	items, _, err := client.DeltaMessages(context.Background(), "", "inbox", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	m := items[0]
	if m.From != "sender@example.com" || m.ReceivedAt != "2026-10-16T05:57:34Z" || !m.IsRead {
		t.Errorf("item = from %q received %q isRead %t, want sender, received time and read state populated", m.From, m.ReceivedAt, m.IsRead)
	}
}
