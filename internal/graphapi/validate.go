package graphapi

import (
	"fmt"
	"regexp"
)

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

// ValidateEmail checks basic email format.
func ValidateEmail(email string) error {
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
