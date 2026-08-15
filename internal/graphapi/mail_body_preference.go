package graphapi

import (
	"fmt"
	"strings"

	abs "github.com/microsoft/kiota-abstractions-go"
	khttp "github.com/microsoft/kiota-http-go"
)

const (
	preferHeader            = "Prefer"
	preferenceAppliedHeader = "Preference-Applied"
)

// BodyPreference is a typed request for the body representation Graph must
// return. The zero value preserves Graph's default.
type BodyPreference string

const (
	BodyDefault BodyPreference = ""
	BodyText    BodyPreference = "text"
	BodyHTML    BodyPreference = "html"

	MessageBodyDefault = BodyDefault
	MessageBodyText    = BodyText
	MessageBodyHTML    = BodyHTML
)

// MessageBodyPreference remains as an alias for existing mail callers.
type MessageBodyPreference = BodyPreference

// ParseBodyPreference converts a CLI-facing value into the closed set accepted
// by the Graph request layer.
func ParseBodyPreference(value string) (BodyPreference, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return BodyDefault, nil
	case string(BodyText):
		return BodyText, nil
	case string(BodyHTML):
		return BodyHTML, nil
	default:
		return BodyDefault, fmt.Errorf("invalid body format %q: must be text or html", value)
	}
}

func ParseMessageBodyPreference(value string) (MessageBodyPreference, error) {
	return ParseBodyPreference(value)
}

func (p BodyPreference) headerValue() (string, error) {
	switch p {
	case BodyDefault:
		return "", nil
	case BodyText, BodyHTML:
		return fmt.Sprintf("outlook.body-content-type=%q", p), nil
	default:
		return "", fmt.Errorf("invalid message body preference %q", p)
	}
}

type bodyResponseContract struct {
	preference BodyPreference
	inspection *khttp.HeadersInspectionOptions
}

func newBodyResponseContract(preference BodyPreference) (*abs.RequestHeaders, []abs.RequestOption, *bodyResponseContract, error) {
	value, err := preference.headerValue()
	if err != nil {
		return nil, nil, nil, err
	}
	if preference == BodyDefault {
		return nil, nil, nil, nil
	}

	headers := abs.NewRequestHeaders()
	headers.Add(preferHeader, value)
	inspection := khttp.NewHeadersInspectionOptions()
	inspection.InspectResponseHeaders = true
	return headers, []abs.RequestOption{inspection}, &bodyResponseContract{
		preference: preference,
		inspection: inspection,
	}, nil
}

func newMessageBodyResponseContract(preference MessageBodyPreference) (*abs.RequestHeaders, []abs.RequestOption, *bodyResponseContract, error) {
	return newBodyResponseContract(preference)
}

func (c *bodyResponseContract) verify() error {
	if c == nil {
		return nil
	}
	return verifyPreferenceApplied(c.inspection.GetResponseHeaders().Get(preferenceAppliedHeader), c.preference)
}

func verifyPreferenceApplied(values []string, preference BodyPreference) error {
	expected, err := preference.headerValue()
	if err != nil {
		return err
	}
	if preference == MessageBodyDefault {
		return nil
	}
	for _, value := range values {
		for directive := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(directive), expected) {
				return nil
			}
		}
	}
	return fmt.Errorf(
		"%s = %q, want exact directive %q",
		preferenceAppliedHeader,
		values,
		expected,
	)
}

func batchResponseHeader(headers map[string]string, name string) []string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return []string{value}
		}
	}
	return nil
}

func verifyMessageBody(message *MailMessage, preference MessageBodyPreference) error {
	if preference == MessageBodyDefault {
		return nil
	}
	if !strings.EqualFold(message.BodyType, string(preference)) {
		return fmt.Errorf("provider body type = %q, want %q", message.BodyType, preference)
	}
	return nil
}
