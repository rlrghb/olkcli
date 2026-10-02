package graphapi

import (
	"html"
	"strings"
)

// plainTextHTML renders plain text as an HTML fragment that keeps its line
// structure. Graph interprets JSON comments as HTML even for plain-text
// originals; escaping must not depend on the original message format. Each line becomes a div and each blank line an empty div holding
// a break, which is the markup Outlook on the web writes for typed text, so
// the spacing survives stylesheets that zero paragraph margins. Leading and
// trailing spaces, and every space that follows another, become non-breaking
// so that indented excerpts keep their columns. One final line terminator is
// treated as ending the last line; any further blank lines are kept.
func plainTextHTML(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return ""
	}
	var out strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			out.WriteString("<div><br></div>")
			continue
		}
		out.WriteString("<div>")
		out.WriteString(preserveSpaces(html.EscapeString(line)))
		out.WriteString("</div>")
	}
	return out.String()
}

func preserveSpaces(escaped string) string {
	var out strings.Builder
	previousSpace := true
	for _, r := range escaped {
		switch {
		case r == '\t':
			out.WriteString("&nbsp;&nbsp;&nbsp;&nbsp;")
			previousSpace = true
		case r == ' ' && previousSpace:
			out.WriteString("&nbsp;")
		case r == ' ':
			out.WriteRune(r)
			previousSpace = true
		default:
			out.WriteRune(r)
			previousSpace = false
		}
	}
	if result, found := strings.CutSuffix(out.String(), " "); found {
		return result + "&nbsp;"
	}
	return out.String()
}
