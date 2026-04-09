package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/rlrghb/olkcli/internal/graphapi"
)

// buildMailFilter builds an OData filter string from common mail filter options
func buildMailFilter(unread bool, from string, after string, before string) (string, error) {
	var filters []string

	if unread {
		filters = append(filters, "isRead eq false")
	}
	if from != "" {
		if err := graphapi.ValidateEmail(from); err != nil {
			return "", fmt.Errorf("invalid --from address: %w", err)
		}
		escaped := strings.ReplaceAll(from, "'", "''")
		filters = append(filters, fmt.Sprintf("from/emailAddress/address eq '%s'", escaped))
	}
	if after != "" {
		canonical := parseDateTime(after)
		if canonical == "" {
			return "", fmt.Errorf("invalid --after date %q: use ISO 8601 format (e.g. 2024-01-15 or 2024-01-15T09:00:00Z)", after)
		}
		filters = append(filters, fmt.Sprintf("receivedDateTime ge %s", canonical))
	}
	if before != "" {
		canonical := parseDateTime(before)
		if canonical == "" {
			return "", fmt.Errorf("invalid --before date %q: use ISO 8601 format (e.g. 2024-01-15 or 2024-01-15T09:00:00Z)", before)
		}
		filters = append(filters, fmt.Sprintf("receivedDateTime le %s", canonical))
	}

	return strings.Join(filters, " and "), nil
}

// parseDateTime validates and returns a canonical ISO 8601 string, or empty if invalid.
func parseDateTime(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format(time.RFC3339)
	}
	return ""
}
