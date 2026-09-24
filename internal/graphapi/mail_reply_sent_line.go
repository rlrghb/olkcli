package graphapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// Graph's createReply writes the quoted "Sent:" line in UTC with a weekday
// and seconds ("Monday, 14 September 2026 07:45:15"), where Outlook on the
// web writes local time without either ("14 September 2026 08:45"). The
// recipient sees that line, so an HTML reply draft rewrites it in the zone
// olk already uses for display. Only Graph's day-month-year rendering is
// recognised; any other layout is left exactly as Graph produced it rather
// than guessed at.
const (
	graphQuotedSentLayout      = "Monday, 2 January 2006 15:04:05"
	outlookWebQuotedSentLayout = "2 January 2006 15:04"
	quotedHeaderMarker         = `id="divRplyFwdMsg"`
	quotedSentLabel            = "<b>Sent:</b>"
)

// quotedSentLineBounds locates the value of the quoted "Sent:" line in the
// outermost divRplyFwdMsg block: the run between the label and the next tag.
// Outlook writes the header lines before any nested element closes, so the
// search stops at the first closing div or nested header after the marker; a
// Sent label past that point belongs to an older reply deeper in the quote
// and keeps what its author's client wrote. The value must be in Graph's layout.
func quotedSentLineBounds(html string) (start, end int, ok bool) {
	header := strings.Index(html, quotedHeaderMarker)
	if header < 0 {
		return 0, 0, false
	}
	block := html[header:]
	if closing := strings.Index(block, "</div>"); closing >= 0 {
		block = block[:closing]
	}
	if nested := strings.Index(block[len(quotedHeaderMarker):], quotedHeaderMarker); nested >= 0 {
		block = block[:len(quotedHeaderMarker)+nested]
	}
	label := strings.Index(block, quotedSentLabel)
	if label < 0 {
		return 0, 0, false
	}
	valueStart := label + len(quotedSentLabel)
	tag := strings.IndexByte(block[valueStart:], '<')
	if tag < 0 {
		return 0, 0, false
	}
	value := strings.TrimSpace(block[valueStart : valueStart+tag])
	if _, err := time.Parse(graphQuotedSentLayout, value); err != nil {
		return 0, 0, false
	}
	start = header + valueStart
	return start, start + tag, true
}

// hasQuotedSentLine reports whether html carries a quoted "Sent:" line that
// rewriteQuotedSentLine would change, so the caller can skip reading the
// original message when there is nothing to rewrite.
func hasQuotedSentLine(html string) bool {
	_, _, ok := quotedSentLineBounds(html)
	return ok
}

// rewriteQuotedSentLine replaces the quoted "Sent:" value with sent, rendered
// in its own location using the web client's layout. The body is returned
// unchanged when it has no recognised Sent line.
func rewriteQuotedSentLine(html string, sent time.Time) string {
	start, end, ok := quotedSentLineBounds(html)
	if !ok {
		return html
	}
	return html[:start] + " " + sent.Format(outlookWebQuotedSentLayout) + html[end:]
}

// messageSentTime reads only the sentDateTime of one message, which Graph
// returns in UTC.
func (c *Client) messageSentTime(ctx context.Context, target, messageID string) (time.Time, error) {
	result, err := c.targetUser(target).Messages().ByMessageId(messageID).Get(ctx, &users.ItemMessagesMessageItemRequestBuilderGetRequestConfiguration{
		Headers:         c.messageIDHeaders(nil),
		QueryParameters: &users.ItemMessagesMessageItemRequestBuilderGetQueryParameters{Select: []string{"sentDateTime"}},
	})
	if err != nil {
		return time.Time{}, c.replyDraftError("reading original message time", target, err)
	}
	if result == nil || result.GetSentDateTime() == nil {
		return time.Time{}, fmt.Errorf("reading original message time: Graph returned no sentDateTime for %s", messageID)
	}
	return *result.GetSentDateTime(), nil
}
