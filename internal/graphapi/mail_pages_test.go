package graphapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	absauth "github.com/microsoft/kiota-abstractions-go/authentication"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TestListMessagesCollectsPagesWithRequestedShape(t *testing.T) {
	requests := 0
	client := testGraphClient(t, func(req *http.Request) *http.Response {
		requests++
		switch requests {
		case 1:
			if req.URL.Path != "/v1.0/users/shared@example.com/mailFolders/inbox/messages" {
				t.Errorf("first request path = %q, want inbox messages", req.URL.Path)
			}
			query := req.URL.Query()
			if got := query.Get("$orderby"); got != "receivedDateTime asc" {
				t.Errorf("first request $orderby = %q, want oldest-first", got)
			}
			if got := query.Get("$select"); got != "id,subject,receivedDateTime" {
				t.Errorf("first request $select = %q, want exact projection", got)
			}
			if got := query.Get("$top"); got != "3" {
				t.Errorf("first request $top = %q, want 3", got)
			}
			return graphJSONResponse(req, `{"value":[{"id":"one","subject":"first"},{"id":"two","subject":"second"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders/inbox/messages?$skiptoken=second"}`)
		case 2:
			if got := req.URL.String(); got != "https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders/inbox/messages?$skiptoken=second" {
				t.Errorf("continuation request URL = %q, want opaque nextLink", got)
			}
			return graphJSONResponse(req, `{"value":[{"id":"three","subject":"third"},{"id":"four","subject":"fourth"}]}`)
		default:
			t.Fatalf("unexpected request %d: %s", requests, req.URL)
			return nil
		}
	})

	messages, err := client.ListMessages(context.Background(), "shared@example.com", &ListMessagesOptions{
		FolderID: "inbox",
		Top:      3,
		OrderBy:  "receivedDateTime asc",
		Select:   []string{"id", "subject", "receivedDateTime"},
	})
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	for index, want := range []string{"one", "two", "three"} {
		if got := messages[index].ID; got != want {
			t.Errorf("message %d ID = %q, want %q", index, got, want)
		}
	}
}

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func testGraphClient(t *testing.T, responder roundTripFunc) *Client {
	t.Helper()
	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		&absauth.AnonymousAuthenticationProvider{}, nil, nil, &http.Client{Transport: responder},
	)
	if err != nil {
		t.Fatalf("creating Graph request adapter: %v", err)
	}
	return &Client{inner: msgraphsdk.NewGraphServiceClient(adapter)}
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

func TestCollectMessagePagesCompletesTwoPages(t *testing.T) {
	first := func(_ context.Context, top int32) (messagePage, error) {
		if top != 3 {
			t.Fatalf("first page top = %d, want 3", top)
		}
		return messagePage{Values: messages("one", "two"), NextLink: "next"}, nil
	}
	next := func(_ context.Context, link string, top int32) (messagePage, error) {
		if link != "next" {
			t.Fatalf("continuation link = %q, want next", link)
		}
		if top != 1 {
			t.Fatalf("continuation top = %d, want 1", top)
		}
		return messagePage{Values: messages("three")}, nil
	}

	got, err := collectMessagePages(context.Background(), 3, first, next)
	if err != nil {
		t.Fatalf("collectMessagePages() error = %v", err)
	}
	assertMessageIDs(t, got, "one", "two", "three")
}

func TestCollectMessagePagesTruncatesFinalPageAtLimit(t *testing.T) {
	first := func(_ context.Context, top int32) (messagePage, error) {
		if top != 3 {
			t.Fatalf("first page top = %d, want 3", top)
		}
		return messagePage{Values: messages("one", "two"), NextLink: "next"}, nil
	}
	next := func(_ context.Context, _ string, top int32) (messagePage, error) {
		if top != 1 {
			t.Fatalf("continuation top = %d, want 1", top)
		}
		return messagePage{Values: messages("three", "four")}, nil
	}

	got, err := collectMessagePages(context.Background(), 3, first, next)
	if err != nil {
		t.Fatalf("collectMessagePages() error = %v", err)
	}
	assertMessageIDs(t, got, "one", "two", "three")
}

func TestCollectMessagePagesReturnsTerminalShortMailbox(t *testing.T) {
	got, err := collectMessagePages(context.Background(), 3,
		func(_ context.Context, top int32) (messagePage, error) {
			if top != 3 {
				t.Fatalf("first page top = %d, want 3", top)
			}
			return messagePage{Values: messages("one", "two")}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			t.Fatal("unexpected continuation request")
			return messagePage{}, nil
		},
	)
	if err != nil {
		t.Fatalf("collectMessagePages() error = %v", err)
	}
	assertMessageIDs(t, got, "one", "two")
}

func TestCollectMessagePagesRejectsDuplicateMessageID(t *testing.T) {
	got, err := collectMessagePages(context.Background(), 2,
		func(context.Context, int32) (messagePage, error) {
			return messagePage{Values: messages("one"), NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			return messagePage{Values: messages("one")}, nil
		},
	)
	if err == nil {
		t.Fatal("collectMessagePages() error = nil, want duplicate rejection")
	}
	if got != nil {
		t.Fatalf("collectMessagePages() messages = %v, want nil on error", got)
	}
}

func TestCollectMessagePagesRejectsEmptyPageWithContinuation(t *testing.T) {
	got, err := collectMessagePages(context.Background(), 2,
		func(context.Context, int32) (messagePage, error) {
			return messagePage{NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			t.Fatal("unexpected continuation request")
			return messagePage{}, nil
		},
	)
	if err == nil {
		t.Fatal("collectMessagePages() error = nil, want zero-progress rejection")
	}
	if got != nil {
		t.Fatalf("collectMessagePages() messages = %v, want nil on error", got)
	}
}

func TestCollectMessagePagesRejectsRepeatedContinuationLink(t *testing.T) {
	got, err := collectMessagePages(context.Background(), 3,
		func(context.Context, int32) (messagePage, error) {
			return messagePage{Values: messages("one"), NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			return messagePage{Values: messages("two"), NextLink: "next"}, nil
		},
	)
	if err == nil {
		t.Fatal("collectMessagePages() error = nil, want repeated continuation rejection")
	}
	if got != nil {
		t.Fatalf("collectMessagePages() messages = %v, want nil on error", got)
	}
}

func TestCollectMessagePagesReturnsNoPartialResultAfterContinuationError(t *testing.T) {
	wantErr := errors.New("continuation failed")
	got, err := collectMessagePages(context.Background(), 2,
		func(context.Context, int32) (messagePage, error) {
			return messagePage{Values: messages("one"), NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			return messagePage{}, wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("collectMessagePages() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("collectMessagePages() messages = %v, want nil on error", got)
	}
}

func TestCollectMessagePagesStopsOnCancellationBeforeContinuation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, err := collectMessagePages(ctx, 2,
		func(context.Context, int32) (messagePage, error) {
			cancel()
			return messagePage{Values: messages("one"), NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			t.Fatal("unexpected continuation request after cancellation")
			return messagePage{}, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("collectMessagePages() error = %v, want context cancellation", err)
	}
	if got != nil {
		t.Fatalf("collectMessagePages() messages = %v, want nil on error", got)
	}
}

func TestCollectMessagePagesStopsAtPageBound(t *testing.T) {
	got, err := collectMessagePages(context.Background(), 2,
		func(_ context.Context, top int32) (messagePage, error) {
			if top != 2 {
				t.Fatalf("first page top = %d, want 2", top)
			}
			return messagePage{Values: messages("one", "two"), NextLink: "next"}, nil
		},
		func(context.Context, string, int32) (messagePage, error) {
			t.Fatal("unexpected continuation request after page bound")
			return messagePage{}, nil
		},
	)
	if err != nil {
		t.Fatalf("collectMessagePages() error = %v", err)
	}
	assertMessageIDs(t, got, "one", "two")
}

func TestValidateGraphContinuation(t *testing.T) {
	scope := continuationScope("graph.microsoft.com", "/v1.0/me/mailFolders/inbox/messages")
	if err := validateGraphContinuation("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages?$skiptoken=abc", scope); err != nil {
		t.Fatalf("validateGraphContinuation() error = %v", err)
	}

	for _, raw := range []string{
		"http://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages",
		"https://user@graph.microsoft.com/v1.0/me/mailFolders/inbox/messages",
		"https://graph.microsoft.com:444/v1.0/me/mailFolders/inbox/messages",
		"https://evil.example.com/v1.0/me/mailFolders/inbox/messages",
		"https://graph.microsoft.com/v1.0/me/mailFolders/archive/messages",
		"https://graph.microsoft.com/v1.0/me/messages",
		"https://graph.microsoft.com/v1.0/users/other@example.com/mailFolders/inbox/messages",
		"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			if err := validateGraphContinuation(raw, scope); err == nil {
				t.Fatal("validateGraphContinuation() error = nil, want rejection")
			}
		})
	}
}

func TestValidateGraphContinuationAllowsDocumentedMailDeltaLinkForms(t *testing.T) {
	tests := []struct {
		name  string
		scope graphContinuationScope
		url   string
	}{
		{
			name:  "me nextLink segment form",
			scope: continuationScope("graph.microsoft.com", "/v1.0/me/mailFolders/AQMk/messages/delta"),
			url:   "https://graph.microsoft.com/v1.0/me/mailFolders/AQMk/messages/delta?$skiptoken=next",
		},
		{
			name:  "me deltaLink alternate key form",
			scope: continuationScope("graph.microsoft.com", "/v1.0/me/mailFolders/AQMk/messages/delta"),
			url:   "https://graph.microsoft.com/v1.0/me/mailfolders('AQMk')/messages/delta?$deltatoken=done",
		},
		{
			name:  "delegated nextLink segment form",
			scope: continuationScope("graph.microsoft.com", "/v1.0/users/shared@example.com/mailFolders/AQMk/messages/delta"),
			url:   "https://graph.microsoft.com/v1.0/users/shared@example.com/mailFolders/AQMk/messages/delta?$skiptoken=next",
		},
		{
			name:  "delegated deltaLink alternate key form",
			scope: continuationScope("graph.microsoft.com", "/v1.0/users/shared@example.com/mailFolders/AQMk/messages/delta"),
			url:   "https://graph.microsoft.com/v1.0/users/shared@example.com/mailfolders('AQMk')/messages/delta?$deltatoken=done",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateGraphContinuation(tc.url, tc.scope); err != nil {
				t.Fatalf("validateGraphContinuation() error = %v", err)
			}
		})
	}
}

func TestValidateGraphContinuationRejectsOtherGraphCloud(t *testing.T) {
	scope := continuationScope("graph.microsoft.com", "/v1.0/me/mailFolders/AQMk/messages/delta")
	if err := validateGraphContinuation("https://graph.microsoft.us/v1.0/me/mailFolders/AQMk/messages/delta?$skiptoken=next", scope); err == nil {
		t.Fatal("validateGraphContinuation() error = nil, want cross-cloud rejection")
	}
}

func TestValidateGraphContinuationBindsExpectedGraphCloud(t *testing.T) {
	for _, tc := range []struct {
		expectedHost string
		actualHost   string
	}{
		{expectedHost: "GRAPH.MICROSOFT.COM", actualHost: "graph.microsoft.com"},
		{expectedHost: "graph.microsoft.us", actualHost: "graph.microsoft.us"},
		{expectedHost: "dod-graph.microsoft.us", actualHost: "dod-graph.microsoft.us"},
		{expectedHost: "microsoftgraph.chinacloudapi.cn", actualHost: "microsoftgraph.chinacloudapi.cn"},
	} {
		t.Run(tc.actualHost, func(t *testing.T) {
			scope := continuationScope(tc.expectedHost, "/v1.0/me/mailFolders/AQMk/messages/delta")
			url := "https://" + tc.actualHost + "/v1.0/me/mailFolders/AQMk/messages/delta?$skiptoken=next"
			if err := validateGraphContinuation(url, scope); err != nil {
				t.Fatalf("validateGraphContinuation() error = %v", err)
			}
		})
	}
}

func messages(ids ...string) []models.Messageable {
	values := make([]models.Messageable, 0, len(ids))
	for _, id := range ids {
		message := models.NewMessage()
		message.SetId(&id)
		values = append(values, message)
	}
	return values
}

func assertMessageIDs(t *testing.T, got []models.Messageable, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d", len(got), len(want))
	}
	for i, message := range got {
		if message.GetId() == nil {
			t.Fatalf("message %d has no ID", i)
		}
		if actual := *message.GetId(); actual != want[i] {
			t.Errorf("message %d ID = %q, want %q", i, actual, want[i])
		}
	}
}

func continuationScope(host, collectionPath string) graphContinuationScope {
	return graphContinuationScope{host: host, collectionPath: collectionPath}
}
