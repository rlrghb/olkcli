package cmd

import (
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

func TestMailListWithoutSelectKeepsDefaultJSON(t *testing.T) {
	_, output := runMailList(t, "--json")
	message := firstJSONMessage(t, output)
	want := []string{"bodyPreview", "categories", "conversationId", "from", "hasAttachments", "id", "isRead", "receivedDateTime", "subject", "to"}
	if got := sortedJSONKeys(message); !reflect.DeepEqual(got, want) {
		t.Errorf("default JSON keys = %v, want %v", got, want)
	}
}

func runMailList(t *testing.T, args ...string) (url.Values, string) {
	t.Helper()
	var query url.Values
	client := testMailListClient(t, func(req *http.Request) *http.Response {
		query = req.URL.Query()
		body := `{
			"value": [{
				"id": "message-id",
				"subject": "Hello",
				"from": {"emailAddress": {"address": "sender@example.com"}},
				"receivedDateTime": "2026-07-28T10:30:00Z",
				"isRead": false,
				"hasAttachments": true,
				"bodyPreview": "Preview",
				"categories": ["green"],
				"conversationId": "conversation-id"
			}]
		}`
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(body)),
			Request:       req,
			ContentLength: int64(len(body)),
		}
	})

	cli := &CLI{}
	parser, err := newKongParser(cli)
	if err != nil {
		t.Fatalf("newKongParser: %v", err)
	}
	kctx, err := parser.Parse(append([]string{"mail", "list"}, args...))
	if err != nil {
		t.Fatalf("parse mail list: %v", err)
	}

	output, _, err := captureStd(func() error {
		return kctx.Run(&RunContext{
			Ctx:    context.Background(),
			Flags:  &cli.RootFlags,
			client: client,
		})
	})
	if err != nil {
		t.Fatalf("run mail list: %v", err)
	}
	return query, output
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
