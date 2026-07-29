package graphapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestParseMessageBodyPreferenceRejectsUnknownRepresentation(t *testing.T) {
	if _, err := ParseMessageBodyPreference("markdown"); err == nil {
		t.Fatal("ParseMessageBodyPreference() error = nil, want unsupported representation rejection")
	}
}

func TestGetMessageRequestsAndVerifiesProviderText(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("Prefer = %q, want provider text preference", got)
		}
		resp := graphJSONResponse(req, `{
			"id":"message-one",
			"body":{"contentType":"text","content":"Provider returned text"}
		}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})

	msg, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyText)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if msg.Body != "Provider returned text" || msg.BodyType != "text" {
		t.Fatalf("GetMessage() body = %q type = %q, want verified provider text", msg.Body, msg.BodyType)
	}
}

func TestGetMessageRequestsAndVerifiesProviderHTML(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="html"` {
			t.Errorf("Prefer = %q, want provider HTML preference", got)
		}
		resp := graphJSONResponse(req, `{
			"id":"message-one",
			"body":{"contentType":"html","content":"<p>Provider returned HTML</p>"}
		}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="html"`)
		return resp
	})

	msg, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyHTML)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if msg.Body != "<p>Provider returned HTML</p>" || msg.BodyType != "html" {
		t.Fatalf("GetMessage() body = %q type = %q, want verified provider HTML", msg.Body, msg.BodyType)
	}
}

func TestGetMessageRejectsMissingProviderAcknowledgement(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		return graphJSONResponse(req, `{
			"id":"message-one",
			"body":{"contentType":"text","content":"Unacknowledged text"}
		}`)
	})

	msg, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "Preference-Applied") {
		t.Fatalf("GetMessage() error = %v, want missing acknowledgement rejection", err)
	}
	if msg != nil {
		t.Fatalf("GetMessage() message = %#v, want nil on representation contract failure", msg)
	}
}

func TestGetMessageAllowsAcknowledgedEmptyProviderBody(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		resp := graphJSONResponse(req, `{"id":"message-one","body":{"contentType":"text","content":""}}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})

	msg, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyText)
	if err != nil {
		t.Fatalf("GetMessage() error = %v, want acknowledged empty body accepted", err)
	}
	if msg.Body != "" || msg.BodyType != "text" {
		t.Fatalf("GetMessage() body = %q type = %q, want empty provider text", msg.Body, msg.BodyType)
	}
}

func TestGetMessageRejectsMismatchedProviderBodyType(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		resp := graphJSONResponse(req, `{"id":"message-one","body":{"contentType":"html","content":"<p>HTML</p>"}}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})

	msg, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "provider body") {
		t.Fatalf("GetMessage() error = %v, want provider body rejection", err)
	}
	if msg != nil {
		t.Fatalf("GetMessage() message = %#v, want nil on representation contract failure", msg)
	}
}

func TestGetMessageDefaultDoesNotRequestOrRequirePreference(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != "" {
			t.Errorf("Prefer = %q, want no provider body preference", got)
		}
		return graphJSONResponse(req, `{"id":"message-one","body":{"contentType":"html","content":""}}`)
	})

	if _, err := client.GetMessage(context.Background(), "", "message-one", MessageBodyDefault); err != nil {
		t.Fatalf("GetMessage() error = %v, want unchanged default behavior", err)
	}
}

func TestGetMessagesBatchRequestsAndVerifiesEveryProviderBody(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if req.URL.Path != "/v1.0/$batch" {
			t.Errorf("request path = %q, want /v1.0/$batch", req.URL.Path)
		}
		var payload struct {
			Requests []struct {
				ID      string            `json:"id"`
				Headers map[string]string `json:"headers"`
			} `json:"requests"`
		}
		data, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("reading batch request: %v", err)
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("decoding batch request: %v\n%s", err, data)
		}
		if len(payload.Requests) != 2 {
			t.Fatalf("batch request count = %d, want 2", len(payload.Requests))
		}
		for _, item := range payload.Requests {
			if got := headerValue(item.Headers, "Prefer"); got != `outlook.body-content-type="text"` {
				t.Errorf("batch item %q Prefer = %q in %v, want provider text preference", item.ID, got, item.Headers)
			}
		}

		return graphJSONResponse(req, `{"responses":[
			{"id":"`+payload.Requests[0].ID+`","status":200,"headers":{"Content-Type":"application/json","Preference-Applied":"outlook.body-content-type=\"text\""},"body":{"id":"one","body":{"contentType":"text","content":""}}},
			{"id":"`+payload.Requests[1].ID+`","status":200,"headers":{"Content-Type":"application/json","Preference-Applied":"outlook.body-content-type=\"text\""},"body":{"id":"two","body":{"contentType":"text","content":"Second"}}}
		]}`)
	})

	messages, err := client.GetMessagesBatch(context.Background(), "", []string{"one", "two"}, MessageBodyText)
	if err != nil {
		t.Fatalf("GetMessagesBatch() error = %v", err)
	}
	got := []string{messages[0].Body, messages[1].Body}
	if !reflect.DeepEqual(got, []string{"", "Second"}) {
		t.Fatalf("GetMessagesBatch() bodies = %v, want [empty Second]", got)
	}
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func TestGetMessagesBatchFailsWholeResultOnRepresentationContractFailure(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		var payload struct {
			Requests []struct {
				ID string `json:"id"`
			} `json:"requests"`
		}
		data, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("decoding batch request: %v", err)
		}
		return graphJSONResponse(req, `{"responses":[
			{"id":"`+payload.Requests[0].ID+`","status":200,"headers":{"Content-Type":"application/json","Preference-Applied":"outlook.body-content-type=\"text\""},"body":{"id":"one","body":{"contentType":"text","content":"First"}}},
			{"id":"`+payload.Requests[1].ID+`","status":200,"headers":{"Content-Type":"application/json"},"body":{"id":"two","body":{"contentType":"text","content":"Second"}}}
		]}`)
	})

	messages, err := client.GetMessagesBatch(context.Background(), "", []string{"one", "two"}, MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "Preference-Applied") {
		t.Fatalf("GetMessagesBatch() error = %v, want whole-call acknowledgement failure", err)
	}
	if messages != nil {
		t.Fatalf("GetMessagesBatch() messages = %#v, want nil before partial output", messages)
	}
}

func TestListThreadRequestsAndVerifiesProviderTextOnEveryPage(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("request %d Prefer = %q, want provider text preference", requests, got)
		}
		if requests == 1 && !strings.Contains(req.URL.Query().Get("$select"), "body") {
			got := req.URL.Query().Get("$select")
			t.Errorf("request %d $select = %q, want explicit body", requests, got)
		}

		var body string
		switch requests {
		case 1:
			body = `{"value":[{"id":"one","receivedDateTime":"2026-01-01T00:00:00Z","body":{"contentType":"text","content":""}}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/messages?$skiptoken=second"}`
		case 2:
			body = `{"value":[{"id":"two","receivedDateTime":"2026-01-02T00:00:00Z","body":{"contentType":"text","content":"Second"}}]}`
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		resp := graphJSONResponse(req, body)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 2, MessageBodyText)
	if err != nil {
		t.Fatalf("ListThread() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	got := []string{messages[0].Body, messages[1].Body}
	if !reflect.DeepEqual(got, []string{"", "Second"}) {
		t.Fatalf("ListThread() bodies = %v, want [empty Second]", got)
	}
}

func TestListThreadRejectsMissingContinuationAcknowledgement(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		var body string
		if requests == 1 {
			body = `{"value":[{"id":"one","body":{"contentType":"text","content":"First"}}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/messages?$skiptoken=second"}`
		} else {
			body = `{"value":[{"id":"two","body":{"contentType":"text","content":"Second"}}]}`
		}
		resp := graphJSONResponse(req, body)
		if requests == 1 {
			resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		}
		return resp
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 2, MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "Preference-Applied") {
		t.Fatalf("ListThread() error = %v, want continuation acknowledgement failure", err)
	}
	if messages != nil {
		t.Fatalf("ListThread() messages = %#v, want nil on representation contract failure", messages)
	}
}
