package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const quotedReplyBody = `<html><body><div>Hi</div><hr><div id="divRplyFwdMsg" dir="ltr">` +
	`<font face="Calibri, sans-serif"><b>From:</b> Maria &lt;m@example.com&gt;<br>` +
	`<b>Sent:</b> Monday, 14 September 2026 07:45:15<br><b>To:</b> Abe<br></font></div>` +
	`<div><div id="divRplyFwdMsg"><b>Sent:</b> Friday, 11 September 2026 16:00:00<br></div></div>` +
	`</body></html>`

func TestRewriteQuotedSentLineRewritesOnlyTheOutermostHeader(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load Europe/London: %v", err)
	}
	sent := time.Date(2026, time.September, 14, 7, 45, 15, 0, time.UTC).In(london)

	got := rewriteQuotedSentLine(quotedReplyBody, sent)

	want := `<html><body><div>Hi</div><hr><div id="divRplyFwdMsg" dir="ltr">` +
		`<font face="Calibri, sans-serif"><b>From:</b> Maria &lt;m@example.com&gt;<br>` +
		`<b>Sent:</b> 14 September 2026 08:45<br><b>To:</b> Abe<br></font></div>` +
		`<div><div id="divRplyFwdMsg"><b>Sent:</b> Friday, 11 September 2026 16:00:00<br></div></div>` +
		`</body></html>`
	if got != want {
		t.Fatalf("rewritten body =\n%s\nwant\n%s", got, want)
	}
}

func TestRewriteQuotedSentLineLeavesBodyAloneWhenNothingToRewrite(t *testing.T) {
	sent := time.Date(2026, time.September, 14, 7, 45, 15, 0, time.UTC)
	tests := []struct {
		name string
		html string
	}{
		{"no quoted header", `<html><body><div>Hi</div></body></html>`},
		{"header without sent label", `<div id="divRplyFwdMsg"><b>From:</b> x<br></div>`},
		{"sent label never closed", `<div id="divRplyFwdMsg"><b>Sent:</b> Monday, 14 September 2026 07:45:15`},
		{"sent label only before the header", `<b>Sent:</b> Monday, 14 September 2026 07:45:15<br><div id="divRplyFwdMsg"></div>`},
		{"sent after the header closes", `<div id="divRplyFwdMsg"><b>From:</b> x<br></div><b>Sent:</b> Monday, 14 September 2026 07:45:15<br>`},
		{"month-first layout", `<div id="divRplyFwdMsg"><b>Sent:</b> Monday, September 14, 2026 7:45:15 AM<br></div>`},
		{"already rewritten", `<div id="divRplyFwdMsg"><b>Sent:</b> 14 September 2026 08:45<br></div>`},
		{"sent only in a nested header", `<div id="divRplyFwdMsg"><b>From:</b> x<br><div id="divRplyFwdMsg"><b>Sent:</b> Friday, 11 September 2026 16:00:00<br></div></div>`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if hasQuotedSentLine(tc.html) {
				t.Fatal("hasQuotedSentLine reported a rewritable line")
			}
			if got := rewriteQuotedSentLine(tc.html, sent); got != tc.html {
				t.Fatalf("body was rewritten:\n%s", got)
			}
		})
	}
}

func quotedReplyDraftResponse(req *http.Request) *http.Response {
	return graphJSONResponse(req, `{"id":"draft-id","subject":"RE: Original","body":{"contentType":"html","content":`+
		string(quotedReplyBodyJSON)+`}}`)
}

func TestCreateReplyDraftWithoutAQuoteLocationKeepsGraphsSentLine(t *testing.T) {
	calls := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		calls++
		switch req.Method {
		case http.MethodPost:
			return quotedReplyDraftResponse(req)
		case http.MethodPatch:
			return graphJSONResponse(req, `{"id":"draft-id","subject":"Re: Original"}`)
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return nil
		}
	})
	draft, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{Body: "<p>Thanks</p>", IsHTML: true})
	if err != nil {
		t.Fatalf("CreateReplyDraft: %v", err)
	}
	if calls != 2 {
		t.Fatalf("Graph requests = %d, want 2 (no read of the original)", calls)
	}
	if !strings.Contains(draft.Body, "<b>Sent:</b> Monday, 14 September 2026 07:45:15<br>") {
		t.Fatalf("Sent line was changed without a location:\n%s", draft.Body)
	}
}

func TestCreateReplyDraftDeletesTheDraftWhenTheSentTimeCannotBeRead(t *testing.T) {
	var deleted bool
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		switch req.Method {
		case http.MethodPost:
			return quotedReplyDraftResponse(req)
		case http.MethodGet:
			if req.URL.Path != "/v1.0/users/team@example.com/messages/AAA" {
				t.Fatalf("original read path = %s", req.URL.Path)
			}
			resp := graphJSONResponse(req, `{"error":{"code":"ErrorItemNotFound","message":"Not found."}}`)
			resp.StatusCode = http.StatusNotFound
			return resp
		case http.MethodDelete:
			deleted = true
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody, Request: req}
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return nil
		}
	})
	_, err := client.CreateReplyDraft(context.Background(), "team@example.com", "AAA", &CreateReplyDraftOptions{
		Body: "<p>Thanks</p>", IsHTML: true, QuoteTimeLocation: time.UTC,
	})
	if err == nil || !strings.Contains(err.Error(), "reading original message time") {
		t.Fatalf("error = %v, want the failed sent-time read", err)
	}
	if !deleted {
		t.Fatal("the incomplete draft was not deleted")
	}
}

var quotedReplyBodyJSON, _ = json.Marshal(quotedReplyBody)

func TestCreateReplyDraftDeletesTheDraftWhenGraphOmitsTheSentTime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		original func(*http.Request) *http.Response
	}{
		{"null sentDateTime", func(req *http.Request) *http.Response {
			return graphJSONResponse(req, `{"id":"AAA","sentDateTime":null}`)
		}},
		{"no message", func(req *http.Request) *http.Response {
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody, Request: req}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSentTimeFailureDeletesDraft(t, tc.original)
		})
	}
}

func assertSentTimeFailureDeletesDraft(t *testing.T, original func(*http.Request) *http.Response) {
	t.Helper()
	var deleted, patched bool
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		switch req.Method {
		case http.MethodPost:
			return quotedReplyDraftResponse(req)
		case http.MethodGet:
			return original(req)
		case http.MethodPatch:
			patched = true
			return graphJSONResponse(req, `{"id":"draft-id"}`)
		case http.MethodDelete:
			deleted = true
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody, Request: req}
		default:
			t.Fatalf("unexpected Graph request: %s %s", req.Method, req.URL.Path)
			return nil
		}
	})
	_, err := client.CreateReplyDraft(context.Background(), "", "AAA", &CreateReplyDraftOptions{
		Body: "<p>Thanks</p>", IsHTML: true, QuoteTimeLocation: time.UTC,
	})
	if err == nil || !strings.Contains(err.Error(), "Graph returned no sentDateTime for AAA") {
		t.Fatalf("error = %v, want the missing sentDateTime", err)
	}
	if patched {
		t.Fatal("the draft was patched after the sent time could not be read")
	}
	if !deleted {
		t.Fatal("the incomplete draft was not deleted")
	}
}
