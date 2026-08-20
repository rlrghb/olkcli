package cmd

import "testing"

// The sending identity is the one thing a dry run cannot afford to omit: a send
// that goes from the wrong mailbox reports success and is only discovered by the
// recipient.
func TestDescribeSender_EmptyTargetNamesOwnMailbox(t *testing.T) {
	if got := describeSender(""); got != "your own mailbox" {
		t.Fatalf("describeSender(%q) = %q, want %q", "", got, "your own mailbox")
	}
}

func TestDescribeSender_TargetIsNamedVerbatim(t *testing.T) {
	if got := describeSender("expenses@example.com"); got != "expenses@example.com" {
		t.Fatalf("describeSender() = %q, want the target address", got)
	}
}

// resolveMailboxTarget is what makes --mailbox reach the send path at all.
// Before this change mail send never called it, so the flag was accepted and
// silently ignored.
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
