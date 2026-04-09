package graphapi

import (
	"context"
	"fmt"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// CalendarEvent is a simplified calendar event for output
type CalendarEvent struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	Organizer   string   `json:"organizer"`
	Attendees   []string `json:"attendees,omitempty"`
	IsAllDay    bool     `json:"isAllDay"`
	IsOnline    bool     `json:"isOnlineMeeting"`
	OnlineURL   string   `json:"onlineMeetingUrl,omitempty"`
	Status      string   `json:"showAs"`
	BodyPreview string   `json:"bodyPreview"`
	Body        string   `json:"body,omitempty"`
}

// CalendarInfo is a simplified calendar representation
type CalendarInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Owner string `json:"owner"`
}

func (c *Client) ListEvents(ctx context.Context, startTime, endTime time.Time, calendarID string, top int32) ([]CalendarEvent, error) {
	top = clampTop(top)

	startStr := startTime.UTC().Format("2006-01-02T15:04:05")
	endStr := endTime.UTC().Format("2006-01-02T15:04:05")

	queryParams := &users.ItemCalendarViewRequestBuilderGetQueryParameters{
		StartDateTime: &startStr,
		EndDateTime:   &endStr,
		Top:           &top,
		Select:        []string{"id", "subject", "start", "end", "location", "organizer", "attendees", "isAllDay", "isOnlineMeeting", "onlineMeetingUrl", "showAs", "bodyPreview"},
		Orderby:       []string{"start/dateTime"},
	}

	var events []CalendarEvent

	if calendarID != "" {
		if err := validateID(calendarID, "calendar ID"); err != nil {
			return nil, err
		}
		calConfig := &users.ItemCalendarsItemCalendarViewRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemCalendarsItemCalendarViewRequestBuilderGetQueryParameters{
				StartDateTime: &startStr,
				EndDateTime:   &endStr,
				Top:           &top,
				Select:        queryParams.Select,
				Orderby:       queryParams.Orderby,
			},
		}
		resp, err := c.inner.Me().Calendars().ByCalendarId(calendarID).CalendarView().Get(ctx, calConfig)
		if err != nil {
			return nil, fmt.Errorf("listing events: %w", err)
		}
		for _, e := range resp.GetValue() {
			events = append(events, convertEvent(e))
		}
		return events, nil
	}

	config := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	resp, err := c.inner.Me().CalendarView().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	for _, e := range resp.GetValue() {
		events = append(events, convertEvent(e))
	}
	return events, nil
}

func (c *Client) GetEvent(ctx context.Context, eventID string) (*CalendarEvent, error) {
	if err := validateID(eventID, "event ID"); err != nil {
		return nil, err
	}
	event, err := c.inner.Me().Events().ByEventId(eventID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting event: %w", err)
	}
	e := convertEvent(event)
	if event.GetBody() != nil && event.GetBody().GetContent() != nil {
		e.Body = *event.GetBody().GetContent()
	}
	return &e, nil
}

func (c *Client) CreateEvent(ctx context.Context, subject string, start, end time.Time, location string, attendees []string, isAllDay bool, isOnlineMeeting bool) (*CalendarEvent, error) {
	event := models.NewEvent()
	event.SetSubject(&subject)

	startDt := models.NewDateTimeTimeZone()
	startStr := start.UTC().Format("2006-01-02T15:04:05")
	startDt.SetDateTime(&startStr)
	utc := "UTC"
	startDt.SetTimeZone(&utc)
	event.SetStart(startDt)

	endDt := models.NewDateTimeTimeZone()
	endStr := end.UTC().Format("2006-01-02T15:04:05")
	endDt.SetDateTime(&endStr)
	endDt.SetTimeZone(&utc)
	event.SetEnd(endDt)

	event.SetIsAllDay(&isAllDay)
	event.SetIsOnlineMeeting(&isOnlineMeeting)

	if location != "" {
		loc := models.NewLocation()
		loc.SetDisplayName(&location)
		event.SetLocation(loc)
	}

	if len(attendees) > 0 {
		var atts []models.Attendeeable
		for _, email := range attendees {
			if err := ValidateEmail(email); err != nil {
				return nil, fmt.Errorf("invalid attendee email: %w", err)
			}
			att := models.NewAttendee()
			addr := models.NewEmailAddress()
			e := email
			addr.SetAddress(&e)
			att.SetEmailAddress(addr)
			required := models.REQUIRED_ATTENDEETYPE
			att.SetTypeEscaped(&required)
			atts = append(atts, att)
		}
		event.SetAttendees(atts)
	}

	created, err := c.inner.Me().Events().Post(ctx, event, nil)
	if err != nil {
		return nil, fmt.Errorf("creating event: %w", err)
	}
	e := convertEvent(created)
	return &e, nil
}

func (c *Client) UpdateEvent(ctx context.Context, eventID string, subject *string, start, end *time.Time, location *string) (*CalendarEvent, error) {
	if err := validateID(eventID, "event ID"); err != nil {
		return nil, err
	}
	event := models.NewEvent()

	if subject != nil {
		event.SetSubject(subject)
	}
	if start != nil {
		startDt := models.NewDateTimeTimeZone()
		startStr := start.UTC().Format("2006-01-02T15:04:05")
		startDt.SetDateTime(&startStr)
		utc := "UTC"
		startDt.SetTimeZone(&utc)
		event.SetStart(startDt)
	}
	if end != nil {
		endDt := models.NewDateTimeTimeZone()
		endStr := end.UTC().Format("2006-01-02T15:04:05")
		endDt.SetDateTime(&endStr)
		utc := "UTC"
		endDt.SetTimeZone(&utc)
		event.SetEnd(endDt)
	}
	if location != nil {
		loc := models.NewLocation()
		loc.SetDisplayName(location)
		event.SetLocation(loc)
	}

	updated, err := c.inner.Me().Events().ByEventId(eventID).Patch(ctx, event, nil)
	if err != nil {
		return nil, fmt.Errorf("updating event: %w", err)
	}
	e := convertEvent(updated)
	return &e, nil
}

func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	if err := validateID(eventID, "event ID"); err != nil {
		return err
	}
	err := c.inner.Me().Events().ByEventId(eventID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}
	return nil
}

func (c *Client) RespondToEvent(ctx context.Context, eventID string, response string) error {
	if err := validateID(eventID, "event ID"); err != nil {
		return err
	}
	switch response {
	case "accept":
		body := users.NewItemEventsItemAcceptPostRequestBody()
		return c.inner.Me().Events().ByEventId(eventID).Accept().Post(ctx, body, nil)
	case "decline":
		body := users.NewItemEventsItemDeclinePostRequestBody()
		return c.inner.Me().Events().ByEventId(eventID).Decline().Post(ctx, body, nil)
	case "tentative":
		body := users.NewItemEventsItemTentativelyAcceptPostRequestBody()
		return c.inner.Me().Events().ByEventId(eventID).TentativelyAccept().Post(ctx, body, nil)
	default:
		return fmt.Errorf("invalid response: %q (must be accept, decline, or tentative)", response)
	}
}

func (c *Client) ListCalendars(ctx context.Context) ([]CalendarInfo, error) {
	resp, err := c.inner.Me().Calendars().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing calendars: %w", err)
	}

	var calendars []CalendarInfo
	for _, cal := range resp.GetValue() {
		ci := CalendarInfo{}
		if cal.GetId() != nil {
			ci.ID = *cal.GetId()
		}
		if cal.GetName() != nil {
			ci.Name = *cal.GetName()
		}
		if cal.GetColor() != nil {
			ci.Color = cal.GetColor().String()
		}
		if cal.GetOwner() != nil && cal.GetOwner().GetAddress() != nil {
			ci.Owner = *cal.GetOwner().GetAddress()
		}
		calendars = append(calendars, ci)
	}
	return calendars, nil
}

func convertEvent(e models.Eventable) CalendarEvent {
	ev := CalendarEvent{}
	if e.GetId() != nil {
		ev.ID = *e.GetId()
	}
	if e.GetSubject() != nil {
		ev.Subject = *e.GetSubject()
	}
	if e.GetStart() != nil && e.GetStart().GetDateTime() != nil {
		ev.Start = *e.GetStart().GetDateTime()
	}
	if e.GetEnd() != nil && e.GetEnd().GetDateTime() != nil {
		ev.End = *e.GetEnd().GetDateTime()
	}
	if e.GetLocation() != nil && e.GetLocation().GetDisplayName() != nil {
		ev.Location = *e.GetLocation().GetDisplayName()
	}
	if e.GetOrganizer() != nil && e.GetOrganizer().GetEmailAddress() != nil && e.GetOrganizer().GetEmailAddress().GetAddress() != nil {
		ev.Organizer = *e.GetOrganizer().GetEmailAddress().GetAddress()
	}
	if e.GetAttendees() != nil {
		for _, a := range e.GetAttendees() {
			if a.GetEmailAddress() != nil && a.GetEmailAddress().GetAddress() != nil {
				ev.Attendees = append(ev.Attendees, *a.GetEmailAddress().GetAddress())
			}
		}
	}
	if e.GetIsAllDay() != nil {
		ev.IsAllDay = *e.GetIsAllDay()
	}
	if e.GetIsOnlineMeeting() != nil {
		ev.IsOnline = *e.GetIsOnlineMeeting()
	}
	if e.GetOnlineMeetingUrl() != nil {
		ev.OnlineURL = *e.GetOnlineMeetingUrl()
	}
	if e.GetShowAs() != nil {
		ev.Status = e.GetShowAs().String()
	}
	if e.GetBodyPreview() != nil {
		ev.BodyPreview = *e.GetBodyPreview()
	}
	return ev
}
