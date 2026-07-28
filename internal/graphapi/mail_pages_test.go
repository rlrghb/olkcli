package graphapi

import (
	"context"
	"errors"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

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
	scope := graphContinuationScope{collectionPath: "/v1.0/me/mailFolders/inbox/messages"}
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
		"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			if err := validateGraphContinuation(raw, scope); err == nil {
				t.Fatal("validateGraphContinuation() error = nil, want rejection")
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
