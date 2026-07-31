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

// MessageBodyPreference is a typed request for the representation Graph must
// return for a message body. The zero value preserves Graph's default.
type MessageBodyPreference string

const (
	MessageBodyDefault MessageBodyPreference = ""
	MessageBodyText    MessageBodyPreference = "text"
	MessageBodyHTML    MessageBodyPreference = "html"
)

// ParseMessageBodyPreference converts a CLI-facing value into the closed set
// accepted by the Graph request layer.
func ParseMessageBodyPreference(value string) (MessageBodyPreference, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return MessageBodyDefault, nil
	case string(MessageBodyText):
		return MessageBodyText, nil
	case string(MessageBodyHTML):
		return MessageBodyHTML, nil
	default:
		return MessageBodyDefault, fmt.Errorf("invalid body format %q: must be text or html", value)
	}
}

func (p MessageBodyPreference) headerValue() (string, error) {
	switch p {
	case MessageBodyDefault:
		return "", nil
	case MessageBodyText, MessageBodyHTML:
		return fmt.Sprintf("outlook.body-content-type=%q", p), nil
	default:
		return "", fmt.Errorf("invalid message body preference %q", p)
	}
}

type messageBodyResponseContract struct {
	preference MessageBodyPreference
	inspection *khttp.HeadersInspectionOptions
}

func newMessageBodyResponseContract(preference MessageBodyPreference) (*abs.RequestHeaders, []abs.RequestOption, *messageBodyResponseContract, error) {
	value, err := preference.headerValue()
	if err != nil {
		return nil, nil, nil, err
	}
	if preference == MessageBodyDefault {
		return nil, nil, nil, nil
	}

	headers := abs.NewRequestHeaders()
	headers.Add(preferHeader, value)
	inspection := khttp.NewHeadersInspectionOptions()
	inspection.InspectResponseHeaders = true
	return headers, []abs.RequestOption{inspection}, &messageBodyResponseContract{
		preference: preference,
		inspection: inspection,
	}, nil
}

func (c *messageBodyResponseContract) verify() error {
	if c == nil {
		return nil
	}
	return verifyPreferenceApplied(c.inspection.GetResponseHeaders().Get(preferenceAppliedHeader), c.preference)
}

func verifyPreferenceApplied(values []string, preference MessageBodyPreference) error {
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
