package cmd

import "testing"

// The mailbox an operation acts on is the one detail whose absence is silent: a
// send leaves from the wrong address and still reports success, and a draft lands
// where the person waiting for it will not look.
func TestDescribeMailbox_EmptyTargetNamesOwnMailbox(t *testing.T) {
	if got := describeMailbox(""); got != "your own mailbox" {
		t.Fatalf("describeMailbox(%q) = %q, want %q", "", got, "your own mailbox")
	}
}

func TestDescribeMailbox_SharedTargetIsNamedVerbatim(t *testing.T) {
	if got := describeMailbox("expenses@example.com"); got != "expenses@example.com" {
		t.Fatalf("describeMailbox() = %q, want the shared address", got)
	}
}

// resolveMailboxTarget is what makes --mailbox reach the send, reply and forward
// paths at all. Before these changes those commands never called it, so the flag
// was accepted and then ignored.
func TestResolveMailboxTarget_FeedsTheSendPath(t *testing.T) {
	target, err := resolveMailboxTarget("expenses@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "expenses@example.com" {
		t.Fatalf("target = %q, want the shared mailbox address", target)
	}
}

func TestResolveMailboxTarget_RejectsNonAddress(t *testing.T) {
	if _, err := resolveMailboxTarget("not-an-address"); err == nil {
		t.Fatal("expected an error for a non-address --mailbox value")
	}
}
