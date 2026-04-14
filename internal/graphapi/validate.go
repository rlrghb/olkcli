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

// safeIDPattern matches typical Microsoft Graph IDs (alphanumeric, hyphens, underscores, equals, plus, slash for base64).
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_=+/-]+$`)

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
// may require a work/school account, if the error is access-denied.
func enterpriseError(action string, err error) error {
	msg := graphErrorMessage(err)
	if strings.Contains(strings.ToLower(msg), "access") && strings.Contains(strings.ToLower(msg), "denied") {
		return fmt.Errorf("%s: %s\n  Note: this feature requires a work/school (Microsoft 365) account and is not available for personal Microsoft accounts (Outlook.com, Hotmail, Live.com)", action, msg)
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
