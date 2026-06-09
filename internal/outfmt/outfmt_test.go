package outfmt

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewPrinter_FormatDispatch(t *testing.T) {
	cases := []struct {
		name       string
		json       bool
		plain      bool
		wantFormat Format
	}{
		{"default is table", false, false, FormatTable},
		{"json flag wins", true, false, FormatJSON},
		{"plain flag", false, true, FormatPlain},
		{"json wins over plain", true, true, FormatJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewPrinter(tc.json, tc.plain, false, "", "UTC", false, false)
			if p.Format != tc.wantFormat {
				t.Fatalf("got format %d, want %d", p.Format, tc.wantFormat)
			}
		})
	}
}

func TestPrintJSON_Envelope(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatJSON, Writer: &buf, Timezone: "America/New_York"}
	results := []map[string]string{{"id": "1"}, {"id": "2"}}
	if err := p.PrintJSON(results, 2, "next-url"); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Count != 2 {
		t.Errorf("count: got %d, want 2", env.Count)
	}
	if env.NextLink != "next-url" {
		t.Errorf("nextLink: got %q, want %q", env.NextLink, "next-url")
	}
	if env.Timezone != "America/New_York" {
		t.Errorf("timezone: got %q, want %q", env.Timezone, "America/New_York")
	}
}

func TestPrintJSON_ResultsOnly(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatJSON, Writer: &buf, ResultsOnly: true}
	results := []int{1, 2, 3}
	if err := p.PrintJSON(results, 3, ""); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	if strings.Contains(buf.String(), "\"results\"") {
		t.Fatalf("ResultsOnly output should not contain envelope; got %s", buf.String())
	}
	var got []int
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d items, want 3", len(got))
	}
}

func TestPrintPlain_TSVNoHeaders(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatPlain, Writer: &buf}
	headers := []string{"id", "subject"}
	rows := [][]string{{"1", "hello"}, {"2", "world"}}
	if err := p.PrintPlain(headers, rows); err != nil {
		t.Fatalf("PrintPlain: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (no header), got %d: %q", len(lines), buf.String())
	}
	if lines[0] != "1\thello" {
		t.Errorf("line 0: got %q, want %q", lines[0], "1\thello")
	}
}

func TestPrintTable_IncludesHeaders(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatTable, Writer: &buf}
	if err := p.PrintTable([]string{"id", "subject"}, [][]string{{"1", "hi"}}); err != nil {
		t.Fatalf("PrintTable: %v", err)
	}
	if !strings.Contains(buf.String(), "id") || !strings.Contains(buf.String(), "subject") {
		t.Errorf("table output missing headers: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hi") {
		t.Errorf("table output missing data row: %q", buf.String())
	}
}

func TestFieldSelection_CaseInsensitive(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatPlain, Writer: &buf, Select: "SUBJECT, id"}
	headers := []string{"id", "subject", "from"}
	rows := [][]string{{"1", "hello", "alice@example.com"}}
	if err := p.PrintPlain(headers, rows); err != nil {
		t.Fatalf("PrintPlain: %v", err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	if line != "hello\t1" {
		t.Errorf("select=subject,id should reorder to those columns; got %q, want %q", line, "hello\t1")
	}
}

func TestFieldSelection_UnknownFieldIgnored(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{Format: FormatPlain, Writer: &buf, Select: "nope,id"}
	headers := []string{"id", "subject"}
	rows := [][]string{{"1", "hi"}}
	if err := p.PrintPlain(headers, rows); err != nil {
		t.Fatalf("PrintPlain: %v", err)
	}
	line := strings.TrimRight(buf.String(), "\n")
	if line != "1" {
		t.Errorf("unknown field should be ignored; got %q, want %q", line, "1")
	}
}

func TestPrint_Dispatches(t *testing.T) {
	cases := []struct {
		name   string
		format Format
		want   string
	}{
		{"json", FormatJSON, "\"results\""},
		{"plain", FormatPlain, "hi"},
		{"table", FormatTable, "subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := &Printer{Format: tc.format, Writer: &buf}
			if err := p.Print([]string{"subject"}, [][]string{{"hi"}}, []map[string]string{{"subject": "hi"}}, 1, ""); err != nil {
				t.Fatalf("Print: %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("format %s output missing %q: %q", tc.name, tc.want, buf.String())
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "hello", "hello"},
		{"newline becomes space", "hello\nworld", "hello world"},
		{"tab becomes space", "hello\tworld", "hello world"},
		{"ansi escape stripped", "\x1b[31mred\x1b[0m", "[31mred[0m"}, // ESC is dropped, brackets/text kept
		{"null byte stripped", "ab\x00cd", "abcd"},
		{"bell stripped", "hi\x07there", "hithere"},
		{"unicode preserved", "café ☕", "café ☕"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.in); got != tc.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeMultiline_PreservesNewlinesAndTabs(t *testing.T) {
	in := "line1\nline2\tcol2\x00\x07"
	want := "line1\nline2\tcol2"
	if got := SanitizeMultiline(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"shorter unchanged", "abc", 10, "abc"},
		{"equal unchanged", "abcde", 5, "abcde"},
		{"longer truncated with ellipsis", "abcdefghij", 6, "abc..."},
		{"max le 3 no ellipsis", "abcdef", 3, "abc"},
		{"max 1", "abcdef", 1, "a"},
		{"unicode rune-safe", "日本語テスト", 4, "日..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Truncate(tc.in, tc.max); got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

func TestConvertTime(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York unavailable: %v", err)
	}

	cases := []struct {
		name string
		in   string
		loc  *time.Location
		want string
	}{
		{"empty passes through", "", ny, ""},
		{"nil loc passes through", "2026-01-15T12:00:00Z", nil, "2026-01-15T12:00:00Z"},
		{"rfc3339 to NY", "2026-01-15T17:30:00Z", ny, "2026-01-15 12:30"},
		{"graph seven-digit fraction", "2026-04-22T15:15:00.0000000", ny, "2026-04-22 11:15"},
		{"date only in UTC", "2026-06-15", time.UTC, "2026-06-15"},
		{"unparseable passes through", "not a date", ny, "not a date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConvertTime(tc.in, tc.loc); got != tc.want {
				t.Errorf("ConvertTime(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
