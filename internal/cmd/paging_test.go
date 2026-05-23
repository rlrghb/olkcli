package cmd

import (
	"strings"
	"testing"
)

func TestResolveMailboxTarget_Empty(t *testing.T) {
	got, err := resolveMailboxTarget("")
	if err != nil {
		t.Fatalf("empty input should be a no-op, got err: %v", err)
	}
	if got != "" {
		t.Errorf("empty input should return empty string, got %q", got)
	}
}

func TestResolveMailboxTarget_TrimsWhitespace(t *testing.T) {
	got, err := resolveMailboxTarget("   ")
	if err != nil {
		t.Fatalf("whitespace-only input should be treated as empty, got err: %v", err)
	}
	if got != "" {
		t.Errorf("whitespace-only input should return empty string, got %q", got)
	}
}

func TestResolveMailboxTarget_Valid(t *testing.T) {
	got, err := resolveMailboxTarget("boss@example.com")
	if err != nil {
		t.Fatalf("valid email should pass, got err: %v", err)
	}
	if got != "boss@example.com" {
		t.Errorf("got %q, want %q", got, "boss@example.com")
	}
}

func TestResolveMailboxTarget_TrimsAroundValid(t *testing.T) {
	got, err := resolveMailboxTarget("  boss@example.com  ")
	if err != nil {
		t.Fatalf("trimmed valid email should pass, got err: %v", err)
	}
	if got != "boss@example.com" {
		t.Errorf("got %q, want %q", got, "boss@example.com")
	}
}

func TestResolveMailboxTarget_Invalid(t *testing.T) {
	cases := []string{
		"not-an-email",
		"@example.com",
		"boss@",
		"boss example.com",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := resolveMailboxTarget(in)
			if err == nil {
				t.Errorf("expected error for %q, got nil", in)
			}
			if err != nil && !strings.Contains(err.Error(), "--mailbox") {
				t.Errorf("error message should mention --mailbox; got %v", err)
			}
		})
	}
}

func TestBuildMailFilter_Empty(t *testing.T) {
	got, err := buildMailFilter(false, "", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("empty inputs should produce empty filter, got %q", got)
	}
}

func TestBuildMailFilter_UnreadOnly(t *testing.T) {
	got, err := buildMailFilter(true, "", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "isRead eq false" {
		t.Errorf("unread-only: got %q", got)
	}
}

func TestBuildMailFilter_FromValid(t *testing.T) {
	got, err := buildMailFilter(false, "alice@example.com", "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "from/emailAddress/address eq 'alice@example.com'" {
		t.Errorf("from-only: got %q", got)
	}
}

func TestBuildMailFilter_FromInvalid(t *testing.T) {
	_, err := buildMailFilter(false, "not-an-email", "", "")
	if err == nil {
		t.Fatalf("invalid email should fail")
	}
	if !strings.Contains(err.Error(), "invalid --from") {
		t.Errorf("error wording should mention --from; got %v", err)
	}
}

func TestBuildMailFilter_QuotesEscaped(t *testing.T) {
	// OData escapes single quotes by doubling them. An apostrophe in the local
	// part of an email is technically valid; we want the resulting filter to
	// not introduce an injection.
	got, err := buildMailFilter(false, "o'connor@example.com", "", "")
	// safeEmailPattern does NOT allow apostrophes, so this should currently
	// reject upstream of the escape step. Document that behavior.
	if err == nil {
		// If ValidateEmail ever loosens, ensure quotes get doubled.
		if !strings.Contains(got, "o''connor") {
			t.Errorf("expected escaped o''connor in filter, got %q", got)
		}
	}
}

func TestBuildMailFilter_DateRange(t *testing.T) {
	got, err := buildMailFilter(false, "", "2026-01-15", "2026-02-15T10:30:00Z")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "receivedDateTime ge 2026-01-15T00:00:00Z") {
		t.Errorf("after filter missing or wrong: %q", got)
	}
	if !strings.Contains(got, "receivedDateTime le 2026-02-15T10:30:00Z") {
		t.Errorf("before filter missing or wrong: %q", got)
	}
	if !strings.Contains(got, " and ") {
		t.Errorf("multiple filters should be joined with ' and ': %q", got)
	}
}

func TestBuildMailFilter_InvalidAfter(t *testing.T) {
	_, err := buildMailFilter(false, "", "yesterday", "")
	if err == nil {
		t.Fatalf("invalid --after should fail")
	}
	if !strings.Contains(err.Error(), "invalid --after") {
		t.Errorf("expected --after error, got %v", err)
	}
}

func TestBuildMailFilter_InvalidBefore(t *testing.T) {
	_, err := buildMailFilter(false, "", "", "tomorrow")
	if err == nil {
		t.Fatalf("invalid --before should fail")
	}
	if !strings.Contains(err.Error(), "invalid --before") {
		t.Errorf("expected --before error, got %v", err)
	}
}

func TestBuildMailFilter_AllCombined(t *testing.T) {
	got, err := buildMailFilter(true, "alice@example.com", "2026-01-15", "2026-02-15")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	parts := []string{
		"isRead eq false",
		"from/emailAddress/address eq 'alice@example.com'",
		"receivedDateTime ge 2026-01-15T00:00:00Z",
		"receivedDateTime le 2026-02-15T00:00:00Z",
	}
	for _, p := range parts {
		if !strings.Contains(got, p) {
			t.Errorf("combined filter missing %q in %q", p, got)
		}
	}
}

func TestParseDateTime(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantNon string // non-empty substring expected, "" means we want parse failure
	}{
		{"rfc3339 with Z", "2026-01-15T09:00:00Z", "2026-01-15T09:00:00Z"},
		{"rfc3339 with offset", "2026-01-15T09:00:00-05:00", "2026-01-15T09:00:00-05:00"},
		{"date+time no zone treated as UTC", "2026-01-15T09:00", "2026-01-15T09:00:00Z"},
		{"date only", "2026-01-15", "2026-01-15T00:00:00Z"},
		{"empty", "", ""},
		{"garbage", "not a date", ""},
		{"month name", "Jan 15 2026", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDateTime(tc.in)
			if tc.wantNon == "" {
				if got != "" {
					t.Errorf("parseDateTime(%q) should reject, got %q", tc.in, got)
				}
				return
			}
			if got != tc.wantNon {
				t.Errorf("parseDateTime(%q) = %q, want %q", tc.in, got, tc.wantNon)
			}
		})
	}
}
