package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	"github.com/rlrghb/olkcli/internal/graphapi"
)

func TestMailListDefaultsToNewestOrder(t *testing.T) {
	query, _ := runMailList(t, "--json")
	if got := query.Get("$orderby"); got != "receivedDateTime desc" {
		t.Errorf("default $orderby = %q, want newest-first", got)
	}
}

func TestMailListAcceptsOldestOrder(t *testing.T) {
	query, _ := runMailList(t, "--json", "--order", "oldest")
	if got := query.Get("$orderby"); got != "receivedDateTime asc" {
		t.Errorf("oldest $orderby = %q, want oldest-first", got)
	}
}

func TestMailListClassificationFiltersUseProviderOrderWhenOrderOmitted(t *testing.T) {
	for _, classification := range []struct {
		flag       string
		wantFilter string
	}{
		{"--focused", "inferenceClassification eq 'focused'"},
		{"--other", "inferenceClassification eq 'other'"},
	} {
		t.Run(classification.flag, func(t *testing.T) {
			query, _ := runMailList(t, "--json", classification.flag)
			if got := query.Get("$orderby"); got != "" {
				t.Errorf("$orderby = %q, want provider default", got)
			}
			if got := query.Get("$filter"); got != classification.wantFilter {
				t.Errorf("$filter = %q, want %q", got, classification.wantFilter)
			}
		})
	}
}

func TestMailListRejectsExplicitOrderWithClassificationWithoutRequest(t *testing.T) {
	for _, classification := range []string{"--focused", "--other"} {
		for _, order := range []string{"newest", "oldest"} {
			t.Run(classification+"/"+order, func(t *testing.T) {
				_, _, calls, err := runMailListResultWithCalls(t, "--json", classification, "--order", order)
				if err == nil || !strings.Contains(err.Error(), "--order cannot be combined with --focused or --other") {
					t.Fatalf("mail list error = %v, want incompatible-order rejection", err)
				}
				if calls != 0 {
					t.Errorf("Graph handler calls = %d, want 0", calls)
				}
			})
		}
	}
}

func TestMailListRejectsInvalidOrder(t *testing.T) {
	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("newKongParser: %v", err)
	}
	if _, err := parser.Parse([]string{"mail", "list", "--order", "middle"}); err == nil {
		t.Fatal("mail list accepted invalid --order")
	}
}

func TestMailListSelectDrivesGraphProjectionAndJSONKeys(t *testing.T) {
	query, output := runMailList(t, "--json", "--select", "id,subject,receivedDateTime")
	if got, want := query.Get("$select"), "id,subject,receivedDateTime"; got != want {
		t.Errorf("$select = %q, want %q", got, want)
	}

	message := firstJSONMessage(t, output)
	if got, want := sortedJSONKeys(message), []string{"id", "receivedDateTime", "subject"}; !reflect.DeepEqual(got, want) {
		t.Errorf("selected JSON keys = %v, want %v", got, want)
	}
}

func TestMailListPlainSelectUsesOutputProjectionOnly(t *testing.T) {
	query, output := runMailList(t, "--plain", "--select", "subject,id")
	if got := query.Get("$select"); got != "id,subject,from,toRecipients,ccRecipients,bccRecipients,replyTo,receivedDateTime,isRead,hasAttachments,bodyPreview,categories,conversationId" {
		t.Errorf("$select = %q, want default mail-list projection", got)
	}
	if got, want := output, "Hello\tmessage-id\n"; got != want {
		t.Errorf("plain selected output = %q, want %q", got, want)
	}
}

func TestMailListAdmittedSelectorsSerialize(t *testing.T) {
	cases := []struct {
		selector string
		jsonKey  string
		want     any
	}{
		{"id", "id", "message-id"},
		{"subject", "subject", "Hello"},
		{"from", "from", "sender@example.com"},
		{"receivedDateTime", "receivedDateTime", "2026-07-28T10:30:00Z"},
		{"isRead", "isRead", false},
		{"hasAttachments", "hasAttachments", true},
		{"bodyPreview", "bodyPreview", "Preview"},
		{"categories", "categories", []any{"green"}},
		{"conversationId", "conversationId", "conversation-id"},
		{"toRecipients", "to", []any{"recipient@example.com"}},
		{"ccRecipients", "cc", []any{"cc@example.com"}},
		{"bccRecipients", "bcc", []any{"bcc@example.com"}},
		{"replyTo", "replyTo", []any{"reply@example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.selector, func(t *testing.T) {
			query, output := runMailList(t, "--json", "--select", tc.selector)
			if got := query.Get("$select"); got != tc.selector {
				t.Errorf("$select = %q, want %q", got, tc.selector)
			}
			message := firstJSONMessage(t, output)
			if got, want := sortedJSONKeys(message), []string{tc.jsonKey}; !reflect.DeepEqual(got, want) {
				t.Errorf("JSON keys = %v, want %v", got, want)
			}
			if got := message[tc.jsonKey]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("JSON %q = %v, want %v", tc.jsonKey, got, tc.want)
			}
		})
	}
}

func TestMailListWithoutSelectKeepsDefaultJSON(t *testing.T) {
	_, output := runMailList(t, "--json")
	message := firstJSONMessage(t, output)
	want := []string{"bcc", "bodyPreview", "categories", "cc", "conversationId", "from", "hasAttachments", "id", "isRead", "receivedDateTime", "replyTo", "subject", "to"}
	if got := sortedJSONKeys(message); !reflect.DeepEqual(got, want) {
		t.Errorf("default JSON keys = %v, want %v", got, want)
	}
}

func TestMailListSelectPreservesUntrustedWrapping(t *testing.T) {
	_, output := runMailList(t, "--json", "--select", "subject", "--wrap-untrusted")
	var envelope struct {
		UntrustedNotice string           `json:"untrustedNotice"`
		Results         []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	if envelope.UntrustedNotice == "" {
		t.Fatal("selected JSON omitted the untrusted-content notice")
	}
	if got, _ := envelope.Results[0]["subject"].(string); !strings.HasPrefix(got, "[UNTRUSTED:") {
		t.Errorf("selected subject = %q, want untrusted-content marker", got)
	}
}

func TestMailListSelectPreservesConciseOutput(t *testing.T) {
	_, output := runMailList(t, "--json", "--select", "bodyPreview", "--concise")
	message := firstJSONMessage(t, output)
	if _, found := message["bodyPreview"]; found {
		t.Errorf("--concise retained selected bodyPreview: %v", message)
	}
}

func TestMailListSelectMapsToRecipientsToCanonicalJSONKey(t *testing.T) {
	query, output := runMailList(t, "--json", "--select", "toRecipients")
	if got := query.Get("$select"); got != "toRecipients" {
		t.Errorf("$select = %q, want toRecipients", got)
	}
	message := firstJSONMessage(t, output)
	if got, want := sortedJSONKeys(message), []string{"to"}; !reflect.DeepEqual(got, want) {
		t.Errorf("toRecipients JSON keys = %v, want %v", got, want)
	}
	if got, want := message["to"], []any{"recipient@example.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("toRecipients JSON value = %v, want %v", got, want)
	}
}

func TestMailListTrimsSelectorWhitespace(t *testing.T) {
	query, output := runMailList(t, "--json", "--select", " id , subject ")
	if got := query.Get("$select"); got != "id,subject" {
		t.Errorf("trimmed $select = %q, want id,subject", got)
	}
	if got, want := sortedJSONKeys(firstJSONMessage(t, output)), []string{"id", "subject"}; !reflect.DeepEqual(got, want) {
		t.Errorf("trimmed selector JSON keys = %v, want %v", got, want)
	}
}

func TestMailListRejectsExplicitEmptySelect(t *testing.T) {
	_, _, calls, err := runMailListResultWithCalls(t, "--json", "--select=")
	if err == nil {
		t.Fatal("mail list accepted explicit empty --select")
	}
	if calls != 0 {
		t.Errorf("Graph handler calls = %d, want 0", calls)
	}
}

func TestMailListRejectsUnserializableGraphSelector(t *testing.T) {
	_, _, calls, err := runMailListResultWithCalls(t, "--json", "--select", "importance")
	if err == nil || !strings.Contains(err.Error(), "not available in mail list output") {
		t.Fatalf("importance selector error = %v, want serializable-output rejection", err)
	}
	if calls != 0 {
		t.Errorf("Graph handler calls = %d, want 0", calls)
	}
}

func TestMailListRejectsUnknownSelector(t *testing.T) {
	_, _, calls, err := runMailListResultWithCalls(t, "--json", "--select", "notAField")
	if err == nil || !strings.Contains(err.Error(), "invalid --select field") {
		t.Fatalf("unknown selector error = %v, want --select validation error", err)
	}
	if calls != 0 {
		t.Errorf("Graph handler calls = %d, want 0", calls)
	}
}

func TestMailListRejectsEmptyAndDuplicateSelectors(t *testing.T) {
	for _, selectFields := range []string{" ", "id, ,subject", "id,id"} {
		t.Run(selectFields, func(t *testing.T) {
			_, _, calls, err := runMailListResultWithCalls(t, "--json", "--select", selectFields)
			if err == nil || !strings.Contains(err.Error(), "--select") {
				t.Fatalf("selector %q error = %v, want local --select validation error", selectFields, err)
			}
			if calls != 0 {
				t.Errorf("Graph handler calls = %d, want 0", calls)
			}
		})
	}
}

func TestMailBatchJSONIgnoresGlobalSelect(t *testing.T) {
	output, calls, err := runMailCommand(t, []string{"mail", "batch"}, []string{"--json", "--select", "subject", "--id", "message-id"}, func(req *http.Request) *http.Response {
		var batch struct {
			Requests []struct {
				ID string `json:"id"`
			} `json:"requests"`
		}
		if err := decodeGraphJSON(req.Body, &batch); err != nil {
			t.Fatalf("decode batch request: %v", err)
		}
		if len(batch.Requests) != 1 {
			t.Fatalf("batch request count = %d, want 1", len(batch.Requests))
		}
		return graphBatchResponse(req, batch.Requests[0].ID)
	})
	if err != nil {
		t.Fatalf("run mail batch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	message := firstJSONMessage(t, output)
	if _, found := message["id"]; !found {
		t.Errorf("mail batch applied mail-list projection: %v", message)
	}
}

func TestMailThreadJSONIgnoresGlobalSelect(t *testing.T) {
	output, calls, err := runMailCommand(t, []string{"mail", "thread"}, []string{"--json", "--select", "subject", "conversation-id"}, graphMessageListResponse)
	if err != nil {
		t.Fatalf("run mail thread: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	message := firstJSONMessage(t, output)
	if _, found := message["id"]; !found {
		t.Errorf("mail thread applied mail-list projection: %v", message)
	}
}

func TestMailThreadCompleteRequestsAnUnboundedProviderTraversal(t *testing.T) {
	output, calls, err := runMailCommand(
		t,
		[]string{"mail", "thread"},
		[]string{"--json", "--complete", "conversation-id"},
		func(req *http.Request) *http.Response {
			if got := req.URL.Query().Get("$top"); got != "1000" {
				t.Errorf("complete thread $top = %q, want page size 1000", got)
			}
			return graphMessageListResponse(req)
		},
	)
	if err != nil {
		t.Fatalf("run complete mail thread: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	if got := firstJSONMessage(t, output)["conversationId"]; got != "conversation-id" {
		t.Fatalf("conversationId = %v, want conversation-id", got)
	}
}

func TestMailGetTextRequestsVerifiedProviderRepresentation(t *testing.T) {
	output, calls, err := runMailCommand(t, []string{"mail", "get"}, []string{"--json", "--format", "text", "message-id"}, func(req *http.Request) *http.Response {
		if got := req.Header.Get("Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("Prefer = %q, want provider text preference", got)
		}
		resp := graphJSONResponse(req, `{"id":"message-id","body":{"contentType":"text","content":"Provider text"}}`)
		resp.Header.Set("Preference-Applied", `outlook.body-content-type="text"`)
		return resp
	})
	if err != nil {
		t.Fatalf("run mail get: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	var envelope struct {
		Results map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	if got := envelope.Results["body"]; got != "Provider text" {
		t.Fatalf("JSON body = %v, want provider text", got)
	}
}

func TestMailBatchBodyFormatTextRequestsVerifiedProviderRepresentation(t *testing.T) {
	output, calls, err := runMailCommand(t, []string{"mail", "batch"}, []string{"--json", "--id", "message-id", "--body-format", "text"}, func(req *http.Request) *http.Response {
		var batch struct {
			Requests []struct {
				ID      string            `json:"id"`
				Headers map[string]string `json:"headers"`
			} `json:"requests"`
		}
		if err := decodeGraphJSON(req.Body, &batch); err != nil {
			t.Fatalf("decode batch request: %v", err)
		}
		if len(batch.Requests) != 1 {
			t.Fatalf("batch request count = %d, want 1", len(batch.Requests))
		}
		if got := caseInsensitiveHeader(batch.Requests[0].Headers, "Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("batch Prefer = %q, want provider text preference", got)
		}
		return graphJSONResponse(req, `{"responses":[{
			"id":"`+batch.Requests[0].ID+`",
			"status":200,
			"headers":{"Content-Type":"application/json","Preference-Applied":"outlook.body-content-type=\"text\""},
			"body":{"id":"message-id","body":{"contentType":"text","content":"Provider text"}}
		}]}`)
	})
	if err != nil {
		t.Fatalf("run mail batch: %v", err)
	}
	if calls != 1 {
		t.Fatalf("Graph handler calls = %d, want 1", calls)
	}
	if got := firstJSONMessage(t, output)["body"]; got != "Provider text" {
		t.Fatalf("JSON body = %v, want provider text", got)
	}
}

func TestMailThreadBodyFormatTextRequestsVerifiedProviderRepresentation(t *testing.T) {
	output, calls, err := runMailCommand(t, []string{"mail", "thread"}, []string{"--json", "--body-format", "text", "conversation-id"}, func(req *http.Request) *http.Response {
		if req.URL.Path != "/v1.0/$batch" {
			if got := req.Header.Get("Prefer"); got != "" {
				t.Errorf("metadata Prefer = %q, want none", got)
			}
			return graphJSONResponse(req, `{"value":[{"id":"message-id"}]}`)
		}
		var batch struct {
			Requests []struct {
				ID      string            `json:"id"`
				Headers map[string]string `json:"headers"`
			} `json:"requests"`
		}
		if err := decodeGraphJSON(req.Body, &batch); err != nil {
			t.Fatalf("decode batch request: %v", err)
		}
		if len(batch.Requests) != 1 {
			t.Fatalf("batch request count = %d, want 1", len(batch.Requests))
		}
		if got := caseInsensitiveHeader(batch.Requests[0].Headers, "Prefer"); got != `outlook.body-content-type="text"` {
			t.Errorf("batch Prefer = %q, want provider text preference", got)
		}
		return graphJSONResponse(req, `{"responses":[{
			"id":"`+batch.Requests[0].ID+`",
			"status":200,
			"headers":{"Content-Type":"application/json","Preference-Applied":"outlook.body-content-type=\"text\""},
			"body":{"id":"message-id","conversationId":"conversation-id","body":{"contentType":"text","content":"Provider text"}}
		}]}`)
	})
	if err != nil {
		t.Fatalf("run mail thread: %v", err)
	}
	if calls != 2 {
		t.Fatalf("Graph handler calls = %d, want 2", calls)
	}
	if got := firstJSONMessage(t, output)["body"]; got != "Provider text" {
		t.Fatalf("JSON body = %v, want provider text", got)
	}
}

func caseInsensitiveHeader(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func runMailList(t *testing.T, args ...string) (query url.Values, output string) {
	t.Helper()
	query, output, err := runMailListResult(t, args...)
	if err != nil {
		t.Fatalf("run mail list: %v", err)
	}
	return query, output
}

func runMailListResult(t *testing.T, args ...string) (query url.Values, output string, err error) {
	query, output, _, err = runMailListResultWithCalls(t, args...)
	return query, output, err
}

func runMailListResultWithCalls(t *testing.T, args ...string) (query url.Values, output string, calls int, err error) {
	t.Helper()
	client := testMailListClient(t, func(req *http.Request) *http.Response {
		calls++
		query = req.URL.Query()
		return graphMessageListResponse(req)
	})

	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		return nil, "", calls, err
	}
	kctx, err := parser.Parse(append([]string{"mail", "list"}, args...))
	if err != nil {
		return nil, "", calls, err
	}

	output, _, err = captureStd(func() error {
		return kctx.Run(&RunContext{
			Ctx:    context.Background(),
			Flags:  &cli.RootFlags,
			client: client,
		})
	})
	return query, output, calls, err
}

func runMailCommand(
	t *testing.T,
	path, args []string,
	responder func(*http.Request) *http.Response,
) (output string, calls int, err error) {
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
	kctx, err := parser.Parse(append(path, args...))
	if err != nil {
		return "", calls, err
	}
	output, _, err = captureStd(func() error {
		return kctx.Run(&RunContext{Ctx: context.Background(), Flags: &cli.RootFlags, client: client})
	})
	return output, calls, err
}

func graphMessageListResponse(req *http.Request) *http.Response {
	body := `{
		"value": [{
			"id": "message-id",
			"subject": "Hello",
			"from": {"emailAddress": {"address": "sender@example.com"}},
			"toRecipients": [{"emailAddress": {"address": "recipient@example.com"}}],
			"ccRecipients": [{"emailAddress": {"address": "cc@example.com"}}],
			"bccRecipients": [{"emailAddress": {"address": "bcc@example.com"}}],
			"replyTo": [{"emailAddress": {"address": "reply@example.com"}}],
			"receivedDateTime": "2026-07-28T10:30:00Z",
			"isRead": false,
			"hasAttachments": true,
			"bodyPreview": "Preview",
			"categories": ["green"],
			"conversationId": "conversation-id"
		}]
	}`
	return graphJSONResponse(req, body)
}

func graphBatchResponse(req *http.Request, stepID string) *http.Response {
	body := `{
		"responses": [{
			"id": "` + stepID + `",
			"status": 200,
			"headers": {"Content-Type": "application/json"},
			"body": {
				"id": "message-id",
				"subject": "Hello",
				"from": {"emailAddress": {"address": "sender@example.com"}},
				"receivedDateTime": "2026-07-28T10:30:00Z",
				"isRead": false,
				"hasAttachments": true,
				"bodyPreview": "Preview",
				"conversationId": "conversation-id"
			}
		}]
	}`
	return graphJSONResponse(req, body)
}

func graphJSONResponse(req *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		Request:       req,
		ContentLength: int64(len(body)),
	}
}

func decodeGraphJSON(reader io.Reader, target any) error {
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer gzipReader.Close()
		return json.NewDecoder(gzipReader).Decode(target)
	}
	return json.Unmarshal(body, target)
}

func testMailListClient(t *testing.T, handler func(*http.Request) *http.Response) *graphapi.Client {
	t.Helper()
	previousTransport := http.DefaultTransport
	http.DefaultTransport = mailListRoundTrip(handler)
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	client, err := graphapi.NewClient(mailListCredential{})
	if err != nil {
		t.Fatalf("new Graph client: %v", err)
	}
	return client
}

type mailListRoundTrip func(*http.Request) *http.Response

func (f mailListRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

type mailListCredential struct{}

func (mailListCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func firstJSONMessage(t *testing.T, output string) map[string]any {
	t.Helper()
	var envelope struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, output)
	}
	if len(envelope.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(envelope.Results))
	}
	return envelope.Results[0]
}

func sortedJSONKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
