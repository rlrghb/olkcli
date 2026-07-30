package graphapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
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

func TestGetMessageAcceptsProviderTextAlongsideImmutableIDPreference(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		got := req.Header.Values("Prefer")
		slices.Sort(got)
		if !slices.Equal(
			got,
			[]string{
				`IdType="ImmutableId"`,
				`outlook.body-content-type="text"`,
			},
		) {
			t.Errorf("Prefer = %q, want provider text and immutable ID", got)
		}
		resp := graphJSONResponse(req, `{
			"id":"immutable-message-one",
			"body":{"contentType":"text","content":"Provider returned text"}
		}`)
		resp.Header.Set(
			"Preference-Applied",
			`outlook.body-content-type="text", IdType="ImmutableId"`,
		)
		return resp
	})
	client.SetImmutableIDs(true)

	msg, err := client.GetMessage(
		context.Background(),
		"",
		"immutable-message-one",
		MessageBodyText,
	)
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if msg.Body != "Provider returned text" || msg.BodyType != "text" {
		t.Fatalf(
			"GetMessage() body = %q type = %q, want verified provider text",
			msg.Body,
			msg.BodyType,
		)
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

func TestListThreadDiscoversPagedMetadataThenFetchesAcknowledgedBatchChunks(t *testing.T) {
	requests := 0
	firstPage := make([]string, 0, 10)
	secondPage := make([]string, 0, 11)
	for index := 20; index >= 11; index-- {
		firstPage = append(firstPage, fmt.Sprintf("message-%02d", index))
	}
	for index := 10; index >= 0; index-- {
		secondPage = append(secondPage, fmt.Sprintf("message-%02d", index))
	}
	var batchSizes []int

	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		switch requests {
		case 1:
			if got := req.Header.Get("Prefer"); got != "" {
				t.Errorf("metadata Prefer = %q, want none", got)
			}
			if got := req.URL.Query().Get("$select"); got != "id" {
				t.Errorf("metadata $select = %q, want id", got)
			}
			return graphMessageIDsResponse(req, firstPage, "https://graph.microsoft.com/v1.0/me/messages?$skiptoken=second")
		case 2:
			if got := req.Header.Get("Prefer"); got != "" {
				t.Errorf("continuation metadata Prefer = %q, want none", got)
			}
			return graphMessageIDsResponse(req, secondPage, "")
		case 3, 4:
			batch := decodeBodyPreferenceBatch(t, req)
			batchSizes = append(batchSizes, len(batch))
			return graphThreadBatchResponse(t, req, batch, "conversation-one", true, nil)
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil
		}
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 21, MessageBodyText)
	if err != nil {
		t.Fatalf("ListThread() error = %v", err)
	}
	if requests != 4 {
		t.Fatalf("request count = %d, want 4", requests)
	}
	if !reflect.DeepEqual(batchSizes, []int{20, 1}) {
		t.Fatalf("batch sizes = %v, want [20 1]", batchSizes)
	}
	if len(messages) != 21 || messages[0].ID != "message-00" || messages[20].ID != "message-20" {
		t.Fatalf("sorted message IDs start/end = %q/%q count=%d, want message-00/message-20 count=21", messages[0].ID, messages[len(messages)-1].ID, len(messages))
	}
}

func TestListCompleteThreadConsumesEveryProviderPage(t *testing.T) {
	requests := 0
	var batchSizes []int
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		switch requests {
		case 1:
			if got := req.URL.Query().Get("$top"); got != "1000" {
				t.Errorf("first metadata $top = %q, want 1000-page request", got)
			}
			if got := req.URL.Query().Get("$select"); got != "id" {
				t.Errorf("metadata $select = %q, want id", got)
			}
			return graphMessageIDsResponse(
				req,
				[]string{"message-02", "message-01"},
				"https://graph.microsoft.com/v1.0/me/messages?$skiptoken=second",
			)
		case 2:
			return graphMessageIDsResponse(
				req,
				[]string{"message-00"},
				"",
			)
		case 3:
			batch := decodeBodyPreferenceBatch(t, req)
			batchSizes = append(batchSizes, len(batch))
			return graphThreadBatchResponse(
				t,
				req,
				batch,
				"conversation-one",
				true,
				nil,
			)
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil
		}
	})

	messages, err := client.ListCompleteThread(
		context.Background(),
		"",
		"conversation-one",
		MessageBodyText,
	)
	if err != nil {
		t.Fatalf("ListCompleteThread() error = %v", err)
	}
	if requests != 3 {
		t.Fatalf("request count = %d, want 3", requests)
	}
	if !reflect.DeepEqual(batchSizes, []int{3}) {
		t.Fatalf("batch sizes = %v, want [3]", batchSizes)
	}
	if len(messages) != 3 ||
		messages[0].ID != "message-00" ||
		messages[2].ID != "message-02" {
		t.Fatalf(
			"sorted message IDs start/end count = %q/%q %d",
			messages[0].ID,
			messages[len(messages)-1].ID,
			len(messages),
		)
	}
}

func TestListThreadRejectsBatchIdentitySetMismatch(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if req.URL.Path != "/v1.0/$batch" {
			return graphMessageIDsResponse(req, []string{"one", "two"}, "")
		}
		batch := decodeBodyPreferenceBatch(t, req)
		return graphThreadBatchResponse(t, req, batch, "conversation-one", true, func(id string) string {
			if id == "two" {
				return "one"
			}
			return id
		})
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 2, MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("ListThread() error = %v, want exact identity rejection", err)
	}
	if messages != nil {
		t.Fatalf("ListThread() messages = %#v, want nil on identity failure", messages)
	}
}

func TestListThreadRejectsWrongConversationInHydratedMessage(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if req.URL.Path != "/v1.0/$batch" {
			return graphMessageIDsResponse(req, []string{"one"}, "")
		}
		return graphThreadBatchResponse(t, req, decodeBodyPreferenceBatch(t, req), "other-conversation", true, nil)
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 1, MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "conversation") {
		t.Fatalf("ListThread() error = %v, want conversation mismatch rejection", err)
	}
	if messages != nil {
		t.Fatalf("ListThread() messages = %#v, want nil on conversation failure", messages)
	}
}

func TestListThreadRejectsUnacknowledgedBatchBody(t *testing.T) {
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		if req.URL.Path != "/v1.0/$batch" {
			return graphMessageIDsResponse(req, []string{"one"}, "")
		}
		return graphThreadBatchResponse(t, req, decodeBodyPreferenceBatch(t, req), "conversation-one", false, nil)
	})

	messages, err := client.ListThread(context.Background(), "", "conversation-one", 1, MessageBodyText)
	if err == nil || !strings.Contains(err.Error(), "Preference-Applied") {
		t.Fatalf("ListThread() error = %v, want batch acknowledgement rejection", err)
	}
	if messages != nil {
		t.Fatalf("ListThread() messages = %#v, want nil on acknowledgement failure", messages)
	}
}

type bodyPreferenceBatchRequest struct {
	ID      string            `json:"id"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func decodeBodyPreferenceBatch(t *testing.T, req *http.Request) []bodyPreferenceBatchRequest {
	t.Helper()
	var payload struct {
		Requests []bodyPreferenceBatchRequest `json:"requests"`
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading batch request: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decoding batch request: %v\n%s", err, data)
	}
	for _, item := range payload.Requests {
		if got := headerValue(item.Headers, "Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("batch item %q Prefer = %q, want provider text preference", item.ID, got)
		}
	}
	return payload.Requests
}

func graphMessageIDsResponse(req *http.Request, ids []string, nextLink string) *http.Response {
	values := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, map[string]string{"id": id})
	}
	payload := map[string]any{"value": values}
	if nextLink != "" {
		payload["@odata.nextLink"] = nextLink
	}
	body, _ := json.Marshal(payload)
	return graphJSONResponse(req, string(body))
}

func graphThreadBatchResponse(
	t *testing.T,
	req *http.Request,
	batch []bodyPreferenceBatchRequest,
	conversationID string,
	acknowledge bool,
	transformID func(string) string,
) *http.Response {
	t.Helper()
	responses := make([]map[string]any, 0, len(batch))
	for _, item := range batch {
		messageID := item.URL
		if marker := strings.LastIndex(messageID, "/messages/"); marker >= 0 {
			messageID = messageID[marker+len("/messages/"):]
		}
		if query := strings.IndexByte(messageID, '?'); query >= 0 {
			messageID = messageID[:query]
		}
		if transformID != nil {
			messageID = transformID(messageID)
		}
		headers := map[string]string{"Content-Type": "application/json"}
		if acknowledge {
			headers["Preference-Applied"] = `outlook.body-content-type="text"`
		}
		received := "2026-01-01T00:00:00Z"
		if suffix := strings.TrimPrefix(messageID, "message-"); suffix != messageID {
			if index, err := strconv.Atoi(suffix); err == nil {
				received = fmt.Sprintf("2026-01-%02dT00:00:00Z", index+1)
			}
		}
		responses = append(responses, map[string]any{
			"id":      item.ID,
			"status":  http.StatusOK,
			"headers": headers,
			"body": map[string]any{
				"id":               messageID,
				"conversationId":   conversationID,
				"receivedDateTime": received,
				"body": map[string]string{
					"contentType": "text",
					"content":     "Provider text for " + messageID,
				},
			},
		})
	}
	body, err := json.Marshal(map[string]any{"responses": responses})
	if err != nil {
		t.Fatalf("encoding batch response: %v", err)
	}
	return graphJSONResponse(req, string(body))
}
