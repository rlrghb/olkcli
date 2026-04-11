package graphapi

import (
	"context"
	"fmt"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// ScheduleInfo represents the free/busy schedule for a user
type ScheduleInfo struct {
	Email        string         `json:"email"`
	Availability string         `json:"availability"`
	Items        []ScheduleItem `json:"scheduleItems"`
}

// ScheduleItem represents a single busy block in a user's schedule
type ScheduleItem struct {
	Status  string `json:"status"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Subject string `json:"subject,omitempty"`
}

// GetSchedule retrieves free/busy availability for the specified email addresses
func (c *Client) GetSchedule(ctx context.Context, emails []string, start, end time.Time) ([]ScheduleInfo, error) {
	for _, email := range emails {
		if err := ValidateEmail(email); err != nil {
			return nil, fmt.Errorf("invalid email: %w", err)
		}
	}

	body := users.NewItemCalendarGetSchedulePostRequestBody()
	body.SetSchedules(emails)

	startDt := models.NewDateTimeTimeZone()
	startStr := start.UTC().Format("2006-01-02T15:04:05")
	startDt.SetDateTime(&startStr)
	tz := graphTimeZoneUTC
	startDt.SetTimeZone(&tz)
	body.SetStartTime(startDt)

	endDt := models.NewDateTimeTimeZone()
	endStr := end.UTC().Format("2006-01-02T15:04:05")
	endDt.SetDateTime(&endStr)
	endDt.SetTimeZone(&tz)
	body.SetEndTime(endDt)

	interval := int32(30)
	body.SetAvailabilityViewInterval(&interval)

	resp, err := c.inner.Me().Calendar().GetSchedule().PostAsGetSchedulePostResponse(ctx, body, nil)
	if err != nil {
		return nil, fmt.Errorf("getting schedule: %w", err)
	}

	var result []ScheduleInfo
	for _, si := range resp.GetValue() {
		info := ScheduleInfo{}
		if si.GetScheduleId() != nil {
			info.Email = *si.GetScheduleId()
		}
		if si.GetAvailabilityView() != nil {
			info.Availability = *si.GetAvailabilityView()
		}
		for _, item := range si.GetScheduleItems() {
			si := ScheduleItem{}
			if item.GetStatus() != nil {
				si.Status = item.GetStatus().String()
			}
			if item.GetStart() != nil && item.GetStart().GetDateTime() != nil {
				si.Start = *item.GetStart().GetDateTime()
			}
			if item.GetEnd() != nil && item.GetEnd().GetDateTime() != nil {
				si.End = *item.GetEnd().GetDateTime()
			}
			if item.GetSubject() != nil {
				si.Subject = *item.GetSubject()
			}
			info.Items = append(info.Items, si)
		}
		result = append(result, info)
	}
	return result, nil
}
