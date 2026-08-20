package graphapi

import (
	"context"
	"strings"
	"testing"
)

// SendMessage must reject a nil options struct rather than send an empty message.
func TestSendMessage_NilOptionsRejected(t *testing.T) {
	c := &Client{}
	err := c.SendMessage(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "send options are required") {
		t.Fatalf("want a nil-options error, got %v", err)
	}
}

// The send guard must still win over everything else, including a target.
func TestSendMessage_NoSendGuardBeatsTarget(t *testing.T) {
	c := &Client{}
	c.SetGuards(false, true)
	err := c.SendMessage(context.Background(), "shared@example.com", &SendMessageOptions{Subject: "s"})
	if err == nil {
		t.Fatal("expected --no-send to block a send to a shared mailbox target")
	}
}

// An invalid importance is rejected before any network call, for a shared
// target just as for the caller's own mailbox.
func TestSendMessage_InvalidImportanceRejected(t *testing.T) {
	c := &Client{}
	err := c.SendMessage(context.Background(), "", &SendMessageOptions{
		Subject:    "s",
		To:         []string{"someone@example.com"},
		Importance: "urgent",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid importance") {
		t.Fatalf("want an importance error, got %v", err)
	}
}
