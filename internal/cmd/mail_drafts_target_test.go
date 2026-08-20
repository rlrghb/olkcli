package cmd

import "testing"

// A draft left in the wrong mailbox is invisible to the person waiting for it,
// so both the dry run and the success line name the mailbox explicitly.
func TestDescribeMailbox_EmptyTargetNamesOwnMailbox(t *testing.T) {
	if got := describeMailbox(""); got != ownMailboxLabel {
		t.Fatalf("describeMailbox(%q) = %q", "", got)
	}
}

func TestDescribeMailbox_SharedTargetIsNamedVerbatim(t *testing.T) {
	if got := describeMailbox("expenses@example.com"); got != "expenses@example.com" {
		t.Fatalf("describeMailbox() = %q, want the shared address", got)
	}
}
