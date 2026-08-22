package graphapi

import (
	"context"
	"strings"
	"testing"
)

// The send guard must win over a target for reply and forward exactly as it does
// for send: a shared mailbox is not a way around --no-send.
func TestReplyMessage_NoSendGuardBeatsTarget(t *testing.T) {
	c := &Client{}
	c.SetGuards(false, true)
	err := c.ReplyMessage(context.Background(), "shared@example.com", "AAA", "body", false)
	if err == nil {
		t.Fatal("expected --no-send to block a reply from a shared mailbox target")
	}
}

func TestForwardMessage_NoSendGuardBeatsTarget(t *testing.T) {
	c := &Client{}
	c.SetGuards(false, true)
	err := c.ForwardMessage(context.Background(), "shared@example.com", "AAA", "", []string{"a@example.com"})
	if err == nil {
		t.Fatal("expected --no-send to block a forward from a shared mailbox target")
	}
}

// A malformed message ID is rejected before any network call, whether or not a
// shared mailbox is targeted.
func TestReplyMessage_InvalidIDRejected(t *testing.T) {
	c := &Client{}
	err := c.ReplyMessage(context.Background(), "shared@example.com", "", "body", false)
	if err == nil || !strings.Contains(err.Error(), "message ID") {
		t.Fatalf("want a message ID error, got %v", err)
	}
}

func TestForwardMessage_InvalidRecipientRejected(t *testing.T) {
	c := &Client{}
	err := c.ForwardMessage(context.Background(), "", "AAA", "", []string{"not-an-address"})
	if err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("want a recipient error, got %v", err)
	}
}

// The grant hint for reply and forward must mention the mailbox-scoped message ID
// trap, which is the failure a delegated reply actually hits first.
func TestReplyGrantHint_NamesBothGrantsAndTheIDTrap(t *testing.T) {
	for _, want := range []string{"Mail.Send.Shared", "Send on Behalf Of", "scoped to a mailbox"} {
		if !strings.Contains(replyGrantHint, want) {
			t.Errorf("replyGrantHint does not mention %q", want)
		}
	}
}
