package graphapi

import (
	"strings"
	"testing"
	"time"
)

func TestValidateID(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		wantErr string // substring expected in error, "" means success
	}{
		{"empty", "", "cannot be empty"},
		{"too long", strings.Repeat("a", 1025), "too long"},
		{"alphanumeric ok", "abc123", ""},
		{"hyphens ok", "abc-123-DEF", ""},
		{"underscore ok", "abc_123", ""},
		{"base64 chars ok", "AAMkAGI=", ""},
		{"onedrive bang ok", "01ABCD!12345", ""},
		{"plus and slash ok", "AAA+BBB/CCC", ""},
		{"space rejected", "abc 123", "invalid characters"},
		{"semicolon rejected", "abc;rm", "invalid characters"},
		{"quote rejected", "abc'", "invalid characters"},
		{"newline rejected", "abc\n", "invalid characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateID(tc.id, "id")
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tc.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	cases := []struct {
		name   string
		email  string
		wantOK bool
	}{
		{"basic ok", "alice@example.com", true},
		{"plus tag ok", "alice+tag@example.com", true},
		{"dots ok", "first.last@sub.example.com", true},
		{"hyphen in domain ok", "u@ex-ample.com", true},
		{"empty", "", false},
		{"no at", "alice.example.com", false},
		{"no tld", "alice@example", false},
		{"trailing space", "alice@example.com ", false},
		{"too long", strings.Repeat("a", 250) + "@a.co", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			if tc.wantOK && err != nil {
				t.Errorf("expected success for %q, got %v", tc.email, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error for %q, got nil", tc.email)
			}
		})
	}
}

func TestValidatePhone(t *testing.T) {
	cases := []struct {
		name   string
		phone  string
		wantOK bool
	}{
		{"plain digits", "5551234567", true},
		{"with hyphens", "555-123-4567", true},
		{"with country code", "+1 (555) 123-4567", true},
		{"with dots", "555.123.4567", true},
		{"empty", "", false},
		{"alpha rejected", "555-CALL", false},
		{"too long", strings.Repeat("1", 31), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePhone(tc.phone)
			if tc.wantOK && err != nil {
				t.Errorf("expected success for %q, got %v", tc.phone, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error for %q, got nil", tc.phone)
			}
		})
	}
}

func TestValidateBirthday(t *testing.T) {
	future := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	cases := []struct {
		name   string
		in     string
		wantOK bool
	}{
		{"valid", "1990-05-15", true},
		{"year 1900 boundary ok", "1900-01-01", true},
		{"too early", "1899-12-31", false},
		{"future date", future, false},
		{"wrong format", "05/15/1990", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateBirthday(tc.in)
			if tc.wantOK && err != nil {
				t.Errorf("expected success for %q, got %v", tc.in, err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error for %q, got nil", tc.in)
			}
		})
	}
}

func TestValidateContactFieldLen(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		limit  int
		wantOK bool
	}{
		{"under limit", "hi", 10, true},
		{"equal limit", "hello", 5, true},
		{"over limit", "hello world", 5, false},
		{"empty under any limit", "", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContactFieldLen(tc.value, "label", tc.limit)
			if tc.wantOK && err != nil {
				t.Errorf("expected success, got %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestClampTop(t *testing.T) {
	cases := []struct {
		in, want int32
	}{
		{0, 25},
		{-5, 25},
		{1, 1},
		{500, 500},
		{1000, 1000},
		{1001, 1000},
		{10000, 1000},
	}
	for _, tc := range cases {
		if got := clampTop(tc.in); got != tc.want {
			t.Errorf("clampTop(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeGraphUTC(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty passes through", "", ""},
		{"unparseable passes through", "not a date", "not a date"},
		{"graph seven-digit fraction", "2026-04-22T15:15:00.0000000", "2026-04-22T15:15:00Z"},
		{"graph no fraction", "2026-04-22T15:15:00", "2026-04-22T15:15:00Z"},
		{"graph minute precision", "2026-04-22T15:15", "2026-04-22T15:15:00Z"},
		{"rfc3339 with Z preserved", "2026-04-22T15:15:00Z", "2026-04-22T15:15:00Z"},
		{"rfc3339 with offset normalized", "2026-04-22T11:15:00-04:00", "2026-04-22T15:15:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeGraphUTC(tc.in); got != tc.want {
				t.Errorf("normalizeGraphUTC(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
