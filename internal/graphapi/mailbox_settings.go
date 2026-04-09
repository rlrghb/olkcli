package graphapi

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// AutoReplySettings is a simplified auto-reply configuration for output
type AutoReplySettings struct {
	Status           string `json:"status"`
	InternalMessage  string `json:"internalMessage"`
	ExternalMessage  string `json:"externalMessage"`
	StartTime        string `json:"startTime,omitempty"`
	EndTime          string `json:"endTime,omitempty"`
	ExternalAudience string `json:"externalAudience"`
}

// GetAutoReply retrieves the current auto-reply (out-of-office) settings
func (c *Client) GetAutoReply(ctx context.Context) (*AutoReplySettings, error) {
	resp, err := c.inner.Me().MailboxSettings().Get(ctx, &users.ItemMailboxSettingsRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailboxSettingsRequestBuilderGetQueryParameters{
			Select: []string{"automaticRepliesSetting"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting mailbox settings: %w", err)
	}

	ars := resp.GetAutomaticRepliesSetting()
	if ars == nil {
		return &AutoReplySettings{Status: "disabled", ExternalAudience: "none"}, nil
	}

	result := &AutoReplySettings{}

	if ars.GetStatus() != nil {
		result.Status = ars.GetStatus().String()
	}
	if ars.GetInternalReplyMessage() != nil {
		result.InternalMessage = *ars.GetInternalReplyMessage()
	}
	if ars.GetExternalReplyMessage() != nil {
		result.ExternalMessage = *ars.GetExternalReplyMessage()
	}
	if ars.GetExternalAudience() != nil {
		result.ExternalAudience = ars.GetExternalAudience().String()
	}
	if ars.GetScheduledStartDateTime() != nil && ars.GetScheduledStartDateTime().GetDateTime() != nil {
		result.StartTime = *ars.GetScheduledStartDateTime().GetDateTime()
	}
	if ars.GetScheduledEndDateTime() != nil && ars.GetScheduledEndDateTime().GetDateTime() != nil {
		result.EndTime = *ars.GetScheduledEndDateTime().GetDateTime()
	}

	return result, nil
}

// SetAutoReply updates the auto-reply (out-of-office) settings
func (c *Client) SetAutoReply(ctx context.Context, status, internalMsg, externalMsg, startTime, endTime, audience string) error {
	autoReply := models.NewAutomaticRepliesSetting()

	// Set status
	switch status {
	case "disabled":
		s := models.DISABLED_AUTOMATICREPLIESSTATUS
		autoReply.SetStatus(&s)
	case "alwaysEnabled":
		s := models.ALWAYSENABLED_AUTOMATICREPLIESSTATUS
		autoReply.SetStatus(&s)
	case "scheduled":
		s := models.SCHEDULED_AUTOMATICREPLIESSTATUS
		autoReply.SetStatus(&s)
	default:
		return fmt.Errorf("invalid status %q: must be disabled, alwaysEnabled, or scheduled", status)
	}

	// Set messages
	if internalMsg != "" {
		autoReply.SetInternalReplyMessage(&internalMsg)
	}
	if externalMsg != "" {
		autoReply.SetExternalReplyMessage(&externalMsg)
	}

	// Set external audience
	switch audience {
	case "none":
		a := models.NONE_EXTERNALAUDIENCESCOPE
		autoReply.SetExternalAudience(&a)
	case "contactsOnly":
		a := models.CONTACTSONLY_EXTERNALAUDIENCESCOPE
		autoReply.SetExternalAudience(&a)
	case "all":
		a := models.ALL_EXTERNALAUDIENCESCOPE
		autoReply.SetExternalAudience(&a)
	default:
		if audience != "" {
			return fmt.Errorf("invalid audience %q: must be none, contactsOnly, or all", audience)
		}
	}

	// Set scheduled times
	if startTime != "" {
		startDt := models.NewDateTimeTimeZone()
		startDt.SetDateTime(&startTime)
		utc := "UTC"
		startDt.SetTimeZone(&utc)
		autoReply.SetScheduledStartDateTime(startDt)
	}
	if endTime != "" {
		endDt := models.NewDateTimeTimeZone()
		endDt.SetDateTime(&endTime)
		utc := "UTC"
		endDt.SetTimeZone(&utc)
		autoReply.SetScheduledEndDateTime(endDt)
	}

	settings := models.NewMailboxSettings()
	settings.SetAutomaticRepliesSetting(autoReply)

	_, err := c.inner.Me().MailboxSettings().Patch(ctx, settings, nil)
	if err != nil {
		return fmt.Errorf("updating auto-reply settings: %w", err)
	}
	return nil
}
