package graphapi

import (
	"context"
	"strings"
	"testing"
)

// These exercise the input-validation paths, which run before any network call,
// so a zero Client (nil inner) is sufficient.

func TestGetMessagesBatch_Validation(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	if _, err := c.GetMessagesBatch(ctx, "", nil, MessageBodyDefault); err == nil {
		t.Error("expected error for empty id list")
	}
	many := make([]string, maxBatchMessages+1)
	for i := range many {
		many[i] = "AAA"
	}
	if _, err := c.GetMessagesBatch(ctx, "", many, MessageBodyDefault); err == nil || !strings.Contains(err.Error(), "max") {
		t.Errorf("expected max-batch error, got %v", err)
	}
	if _, err := c.GetMessagesBatch(ctx, "", []string{"bad id with spaces"}, MessageBodyDefault); err == nil {
		t.Error("expected invalid-id error")
	}
}

func TestListThread_Validation(t *testing.T) {
	c := &Client{}
	if _, err := c.ListThread(context.Background(), "", "", 50, MessageBodyDefault); err == nil {
		t.Error("expected error for empty conversation id")
	}
	// A quote can't pass validateID, so OData injection is impossible.
	if _, err := c.ListThread(context.Background(), "", "x' or '1'='1", 50, MessageBodyDefault); err == nil {
		t.Error("expected invalid-id error for quote-bearing conversation id")
	}
}
