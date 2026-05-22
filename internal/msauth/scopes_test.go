package msauth

import (
	"reflect"
	"testing"
)

func TestMergeScopes_EmptyExtras(t *testing.T) {
	base := []string{"offline_access", "User.Read", "Mail.ReadWrite"}
	got := MergeScopes(base, nil)
	if !reflect.DeepEqual(got, base) {
		t.Errorf("got %v, want %v", got, base)
	}
}

func TestMergeScopes_AppendsNew(t *testing.T) {
	base := []string{"User.Read", "Mail.ReadWrite"}
	extras := []string{"Mail.Read.Shared", "Calendars.Read.Shared"}
	got := MergeScopes(base, extras)
	want := []string{"User.Read", "Mail.ReadWrite", "Mail.Read.Shared", "Calendars.Read.Shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeScopes_DedupCaseInsensitive(t *testing.T) {
	base := []string{"User.Read", "Mail.ReadWrite"}
	extras := []string{"mail.readwrite", "Mail.Read.Shared", "MAIL.READ.SHARED"}
	got := MergeScopes(base, extras)
	want := []string{"User.Read", "Mail.ReadWrite", "Mail.Read.Shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeScopes_FirstCasingWins(t *testing.T) {
	got := MergeScopes([]string{"Mail.ReadWrite"}, []string{"mail.readwrite"})
	want := []string{"Mail.ReadWrite"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v — first occurrence's casing should win", got, want)
	}
}

func TestMergeScopes_TrimsWhitespace(t *testing.T) {
	got := MergeScopes(nil, []string{"  Mail.Read.Shared  ", "\tCalendars.Read.Shared\n"})
	want := []string{"Mail.Read.Shared", "Calendars.Read.Shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeScopes_DropsEmpty(t *testing.T) {
	got := MergeScopes([]string{"User.Read", ""}, []string{"", "  ", "Mail.Read.Shared"})
	want := []string{"User.Read", "Mail.Read.Shared"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergeScopes_DoesNotMutateBase(t *testing.T) {
	base := []string{"User.Read", "Mail.ReadWrite"}
	baseCopy := append([]string(nil), base...)
	_ = MergeScopes(base, []string{"Mail.Read.Shared"})
	if !reflect.DeepEqual(base, baseCopy) {
		t.Errorf("base was mutated: got %v, want %v", base, baseCopy)
	}
}

func TestDefaultScopes_IncludesEssentials(t *testing.T) {
	got := DefaultScopes()
	required := []string{ScopeOfflineAccess, ScopeUser, ScopeMail, ScopeMailSend}
	for _, r := range required {
		found := false
		for _, s := range got {
			if s == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DefaultScopes missing %q in %v", r, got)
		}
	}
}

func TestEnterpriseScopes_ExtendsDefault(t *testing.T) {
	def := DefaultScopes()
	ent := EnterpriseScopes()
	if len(ent) <= len(def) {
		t.Errorf("EnterpriseScopes should extend DefaultScopes, got %d vs %d", len(ent), len(def))
	}
	hasUserReadAll := false
	hasMailboxSettings := false
	for _, s := range ent {
		switch s {
		case ScopeUserReadAll:
			hasUserReadAll = true
		case ScopeMailboxSettings:
			hasMailboxSettings = true
		}
	}
	if !hasUserReadAll || !hasMailboxSettings {
		t.Errorf("EnterpriseScopes missing enterprise-only scopes: %v", ent)
	}
}
