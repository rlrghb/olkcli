package graphapi

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// graphTimeZoneUTC is the time zone string Microsoft Graph expects on dateTimeTimeZone values.
const graphTimeZoneUTC = "UTC"

// graphDateTimeFormats are the layouts Microsoft Graph returns on dateTimeTimeZone.dateTime fields.
// The values are UTC wall-clock times without a zone suffix, so we parse as UTC and re-emit as RFC3339.
var graphDateTimeFormats = []string{
	"2006-01-02T15:04:05.0000000",
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	time.RFC3339Nano,
	time.RFC3339,
}

// normalizeGraphUTC converts a Microsoft Graph dateTimeTimeZone.dateTime string to RFC3339 with a Z suffix.
// Graph returns values like "2026-04-22T15:15:00.0000000" (UTC wall-clock, no zone), which JSON clients
// misinterpret as local time. We always request timeZone=UTC, so it is safe to force Z here. Returns
// the input unchanged if empty or unparseable.
func normalizeGraphUTC(s string) string {
	if s == "" {
		return s
	}
	for _, layout := range graphDateTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return s
}

// safeIDPattern matches typical Microsoft Graph IDs (alphanumeric, hyphens, underscores, equals, plus, slash for base64, exclamation for OneDrive).
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_=+/!-]+$`)

// validateID checks that an ID parameter contains only safe characters.
func validateID(id, label string) error {
	if id == "" {
		return fmt.Errorf("%s cannot be empty", label)
	}
	if len(id) > 1024 {
		return fmt.Errorf("%s too long", label)
	}
	if !safeIDPattern.MatchString(id) {
		return fmt.Errorf("%s contains invalid characters", label)
	}
	return nil
}

// ValidateEmail checks basic email format and length.
func ValidateEmail(email string) error {
	if len(email) > 254 {
		return fmt.Errorf("email address too long: %d characters (max 254)", len(email))
	}
	if !safeEmailPattern.MatchString(email) {
		return fmt.Errorf("invalid email address: %q", email)
	}
	return nil
}

// safePhonePattern matches common phone number formats.
var safePhonePattern = regexp.MustCompile(`^[0-9 ()+.\-]{1,30}$`)

// ValidatePhone checks basic phone number format.
func ValidatePhone(phone string) error {
	if !safePhonePattern.MatchString(phone) {
		return fmt.Errorf("invalid phone number: %q", phone)
	}
	return nil
}

// ValidateBirthday parses an ISO date string (YYYY-MM-DD) and returns the parsed time.
func ValidateBirthday(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid birthday %q: use YYYY-MM-DD format", s)
	}
	if t.Year() < 1900 || t.After(time.Now()) {
		return time.Time{}, fmt.Errorf("invalid birthday %q: must be between 1900 and today", s)
	}
	return t, nil
}

// maxContactFieldLen is the maximum length for general contact string fields.
const maxContactFieldLen = 255

// maxContactNotesLen is the maximum length for the PersonalNotes field.
const maxContactNotesLen = 32000

// ValidateContactFieldLen checks that a contact string field is within length limits.
func ValidateContactFieldLen(value, label string, limit int) error {
	if len(value) > limit {
		return fmt.Errorf("%s too long: %d characters (max %d)", label, len(value), limit)
	}
	return nil
}

// graphErrorMessage extracts a readable message from a Graph API error.
func graphErrorMessage(err error) string {
	var odataErr *odataerrors.ODataError
	if errors.As(err, &odataErr) {
		if main := odataErr.GetErrorEscaped(); main != nil {
			msg := ""
			if main.GetCode() != nil {
				msg = *main.GetCode()
			}
			if main.GetMessage() != nil && *main.GetMessage() != "" {
				if msg != "" {
					msg += ": "
				}
				msg += *main.GetMessage()
			}
			// Check error details for additional context
			for _, d := range main.GetDetails() {
				if d.GetCode() != nil || d.GetMessage() != nil {
					detail := ""
					if d.GetCode() != nil {
						detail = *d.GetCode()
					}
					if d.GetMessage() != nil && *d.GetMessage() != "" {
						if detail != "" {
							detail += ": "
						}
						detail += *d.GetMessage()
					}
					if detail != "" {
						msg += " (" + detail + ")"
					}
				}
			}
			if msg != "" {
				return msg
			}
		}
	}
	if err != nil {
		s := err.Error()
		if s != "" {
			return s
		}
	}
	return "unknown error"
}

// enterpriseError wraps a Graph API error with a hint that the feature
// may require a work/school account, if the error indicates access issues.
func enterpriseError(action string, err error) error {
	msg := graphErrorMessage(err)
	lower := strings.ToLower(msg)
	needsEnterprise := (strings.Contains(lower, "access") && strings.Contains(lower, "denied")) ||
		lower == "unknownerror" ||
		strings.Contains(lower, "mailboxnotenabledforrestapi")
	if needsEnterprise {
		return fmt.Errorf("%s: %s\n  Note: this feature requires a work/school (Microsoft 365) account and is not available for personal Microsoft accounts (Outlook.com, Hotmail, Live.com)", action, msg)
	}
	return fmt.Errorf("%s: %s", action, msg)
}

// scopeUpgradeError wraps a Graph API error with a hint to re-login
// when the error indicates missing permissions (e.g. token lacks a newly added scope).
func scopeUpgradeError(action string, err error) error {
	msg := graphErrorMessage(err)
	lower := strings.ToLower(msg)
	needsReauth := strings.Contains(lower, "accessdenied") ||
		strings.Contains(lower, "insufficient") ||
		(strings.Contains(lower, "access") && strings.Contains(lower, "denied")) ||
		strings.Contains(lower, "authorization_requestdenied")
	if needsReauth {
		return fmt.Errorf("%s: %s\n  Hint: you may need to re-login to grant new permissions: olk auth login", action, msg)
	}
	return fmt.Errorf("%s: %s", action, msg)
}

// clampTop normalizes the top/limit parameter to a safe range.
func clampTop(top int32) int32 {
	if top <= 0 {
		return 25
	}
	if top > 1000 {
		return 1000
	}
	return top
}
