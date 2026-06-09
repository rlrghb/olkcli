package outfmt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

type Format int

const (
	FormatTable Format = iota
	FormatJSON
	FormatPlain // TSV
)

// Envelope is the JSON output wrapper. UntrustedNotice is emitted first (and
// only when --wrap-untrusted is set) so a consuming LLM reads the security
// directive before the wrapped, externally-controlled results.
type Envelope struct {
	UntrustedNotice string      `json:"untrustedNotice,omitempty"`
	Results         interface{} `json:"results"`
	Count           int         `json:"count"`
	NextLink        string      `json:"nextLink,omitempty"`
	// Delta-sync continuation (set only by delta/changes commands). DeltaToken is
	// the opaque cursor to pass back next time; DeltaComplete reports whether the
	// caller is caught up (token is a deltaLink) or more pages remain now.
	DeltaToken    string `json:"deltaToken,omitempty"`
	DeltaComplete *bool  `json:"deltaComplete,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
}

// Printer handles output formatting
type Printer struct {
	Format        Format
	Writer        io.Writer
	Select        string // comma-separated field names
	ResultsOnly   bool
	Timezone      string // IANA timezone name for JSON envelope metadata
	WrapUntrusted bool   // wrap externally-controlled free-text fields in JSON output
	Concise       bool   // drop large free-text fields (bodies, previews, attendees) from JSON output
}

// NewPrinter creates a printer from flags
func NewPrinter(jsonFlag, plainFlag, resultsOnly bool, selectFields, timezone string, wrapUntrusted, concise bool) *Printer {
	f := FormatTable
	if jsonFlag {
		f = FormatJSON
	} else if plainFlag {
		f = FormatPlain
	}
	return &Printer{
		Format:        f,
		Writer:        os.Stdout,
		Select:        selectFields,
		ResultsOnly:   resultsOnly,
		Timezone:      timezone,
		WrapUntrusted: wrapUntrusted,
		Concise:       concise,
	}
}

// PrintJSON outputs data as JSON with envelope. When WrapUntrusted is set,
// fields tagged `untrusted:"true"` are wrapped with markers so an LLM/agent
// consumer can distinguish externally-controlled text from trusted output.
func (p *Printer) PrintJSON(results interface{}, count int, nextLink string) error {
	if p.Concise {
		results = dropConcise(results)
	}
	notice := ""
	if p.WrapUntrusted {
		id := newUntrustedID()
		results = wrapUntrusted(results, id)
		notice = untrustedNotice(id)
	}
	enc := json.NewEncoder(p.Writer)
	enc.SetIndent("", "  ")
	if p.ResultsOnly {
		// No envelope to carry the notice; the id-scoped markers remain on the
		// wrapped fields, but the directive is dropped (advanced opt-out).
		return enc.Encode(results)
	}
	return enc.Encode(Envelope{
		UntrustedNotice: notice,
		Results:         results,
		Count:           count,
		NextLink:        nextLink,
		Timezone:        p.Timezone,
	})
}

// PrintDelta renders a delta-sync page: in JSON mode it emits the envelope with
// the continuation token and completeness (results still wrapped/concised like
// PrintJSON); in table/plain mode it prints the rows followed by the token and
// complete flag so a human/script can resume.
func (p *Printer) PrintDelta(headers []string, rows [][]string, jsonData interface{}, count int, token string, complete bool) error {
	if p.Format == FormatJSON {
		results := jsonData
		if p.Concise {
			results = dropConcise(results)
		}
		notice := ""
		if p.WrapUntrusted {
			id := newUntrustedID()
			results = wrapUntrusted(results, id)
			notice = untrustedNotice(id)
		}
		enc := json.NewEncoder(p.Writer)
		enc.SetIndent("", "  ")
		if p.ResultsOnly {
			return enc.Encode(results)
		}
		return enc.Encode(Envelope{
			UntrustedNotice: notice,
			Results:         results,
			Count:           count,
			DeltaToken:      token,
			DeltaComplete:   &complete,
			Timezone:        p.Timezone,
		})
	}

	if p.Format == FormatPlain {
		if err := p.PrintPlain(headers, rows); err != nil {
			return err
		}
	} else if err := p.PrintTable(headers, rows); err != nil {
		return err
	}
	_, err := fmt.Fprintf(p.Writer, "\nDelta token: %s\nComplete: %v\n", token, complete)
	return err
}

// PrintTable outputs data as an aligned table
func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	selected := p.selectedFields(headers)
	w := tabwriter.NewWriter(p.Writer, 0, 0, 2, ' ', 0)

	if selected != nil {
		headers = filterFields(headers, selected)
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	for _, row := range rows {
		if selected != nil {
			row = filterFields(row, selected)
		}
		fmt.Fprintln(w, strings.Join(sanitizeRow(row), "\t"))
	}
	return w.Flush()
}

// PrintPlain outputs data as TSV (no headers)
func (p *Printer) PrintPlain(headers []string, rows [][]string) error {
	selected := p.selectedFields(headers)
	for _, row := range rows {
		if selected != nil {
			row = filterFields(row, selected)
		}
		fmt.Fprintln(p.Writer, strings.Join(sanitizeRow(row), "\t"))
	}
	return nil
}

// Print dispatches to the appropriate format
func (p *Printer) Print(headers []string, rows [][]string, jsonData interface{}, count int, nextLink string) error {
	switch p.Format {
	case FormatJSON:
		return p.PrintJSON(jsonData, count, nextLink)
	case FormatPlain:
		return p.PrintPlain(headers, rows)
	case FormatTable:
		return p.PrintTable(headers, rows)
	default:
		// Format is an int; treat unknown values like table output.
		return p.PrintTable(headers, rows)
	}
}

func (p *Printer) selectedFields(headers []string) []int {
	if p.Select == "" {
		return nil
	}
	fields := strings.Split(p.Select, ",")
	var indices []int
	for _, f := range fields {
		f = strings.TrimSpace(f)
		for i, h := range headers {
			if strings.EqualFold(h, f) {
				indices = append(indices, i)
				break
			}
		}
	}
	return indices
}

func filterFields(row []string, indices []int) []string {
	result := make([]string, len(indices))
	for i, idx := range indices {
		if idx < len(row) {
			result[i] = row[idx]
		}
	}
	return result
}

// sanitizeRow strips ANSI escape sequences and control characters from all
// fields in a row before printing to the terminal. This prevents malicious
// data (e.g., crafted email subjects) from manipulating the user's terminal.
func sanitizeRow(row []string) []string {
	out := make([]string, len(row))
	for i, s := range row {
		out[i] = Sanitize(s)
	}
	return out
}

// Sanitize removes control characters from a string, replacing newlines
// with spaces (safe for single-line table cells). Use SanitizeMultiline
// for free-text body output where newlines should be preserved.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return ' ' // replace newline with space for table cells
		}
		if r == '\t' {
			return ' ' // replace tab with space for table cells
		}
		if unicode.IsControl(r) {
			return -1 // drop
		}
		return r
	}, s)
}

// Truncate truncates a string to maxRunes runes, appending "..." if truncated.
// Unlike byte-level slicing, this is safe for multi-byte UTF-8 characters.
func Truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

// SanitizeMultiline removes control characters but preserves newlines,
// suitable for multi-line body text output.
func SanitizeMultiline(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1 // drop
		}
		return r
	}, s)
}

// ConvertTime parses a UTC time string and converts it to the given location.
// For date-only inputs, returns "2006-01-02". For datetime inputs, returns
// "2006-01-02 15:04". Returns the original string unchanged if loc is nil or
// parsing fails.
func ConvertTime(utcStr string, loc *time.Location) string {
	if loc == nil || utcStr == "" {
		return utcStr
	}

	formats := []struct {
		layout   string
		dateOnly bool
	}{
		{time.RFC3339, false},
		{"2006-01-02T15:04:05.0000000", false},
		{"2006-01-02T15:04:05", false},
		{"2006-01-02T15:04", false},
		{"2006-01-02", true},
	}

	for _, f := range formats {
		t, err := time.Parse(f.layout, utcStr)
		if err != nil {
			continue
		}
		if f.dateOnly {
			return t.In(loc).Format("2006-01-02")
		}
		return t.In(loc).Format("2006-01-02 15:04")
	}

	return utcStr
}
