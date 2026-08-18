package graphapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/microsoft/kiota-abstractions-go/serialization"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var dayNameTitle = cases.Title(language.English)

// CalendarEvent is a simplified calendar event for output
type CalendarEvent struct {
	ID                string               `json:"id"`
	Subject           string               `json:"subject" untrusted:"true"`
	Start             string               `json:"start"`
	End               string               `json:"end"`
	Location          string               `json:"location" untrusted:"true"`
	Organizer         string               `json:"organizer" untrusted:"true"`
	Attendees         []string             `json:"attendees,omitempty" untrusted:"true" concise:"omit"`
	IsAllDay          bool                 `json:"isAllDay"`
	IsOnline          bool                 `json:"isOnlineMeeting"`
	OnlineURL         string               `json:"onlineMeetingUrl,omitempty" untrusted:"true"`
	Status            string               `json:"showAs"`
	Recurrence        string               `json:"recurrence,omitempty"`
	BodyPreview       string               `json:"bodyPreview,omitempty" untrusted:"true" concise:"omit"`
	Body              string               `json:"body,omitempty" untrusted:"true" concise:"omit"`
	BodyType          string               `json:"bodyType,omitempty"`
	ICalUID           string               `json:"iCalUId,omitempty"`
	ChangeKey         string               `json:"changeKey,omitempty"`
	CreatedAt         string               `json:"createdDateTime,omitempty"`
	ModifiedAt        string               `json:"lastModifiedDateTime,omitempty"`
	EventType         string               `json:"type,omitempty"`
	SeriesMasterID    string               `json:"seriesMasterId,omitempty"`
	OriginalStart     string               `json:"originalStart,omitempty"`
	IsCancelled       bool                 `json:"isCancelled"`
	IsReminderOn      *bool                `json:"isReminderOn,omitempty"`
	TransactionID     string               `json:"transactionId,omitempty"`
	AttendeeDetails   []AttendeeDetails    `json:"attendeeDetails,omitempty" concise:"omit"`
	ResponseStatus    *EventResponseStatus `json:"responseStatus,omitempty" concise:"omit"`
	RecurrenceDetails *RecurrenceDetails   `json:"recurrenceDetails,omitempty" concise:"omit"`
}

type AttendeeDetails struct {
	Name         string `json:"name,omitempty" untrusted:"true"`
	Address      string `json:"address,omitempty" untrusted:"true"`
	Type         string `json:"type,omitempty"`
	Response     string `json:"response,omitempty"`
	ResponseTime string `json:"responseTime,omitempty"`
}
type EventResponseStatus struct {
	Response string `json:"response,omitempty"`
	Time     string `json:"time,omitempty"`
}
type RecurrenceDetails struct {
	PatternType         string   `json:"patternType,omitempty"`
	Interval            int32    `json:"interval,omitempty"`
	DaysOfWeek          []string `json:"daysOfWeek,omitempty"`
	DayOfMonth          int32    `json:"dayOfMonth,omitempty"`
	Month               int32    `json:"month,omitempty"`
	Index               string   `json:"index,omitempty"`
	RangeType           string   `json:"rangeType,omitempty"`
	StartDate           string   `json:"startDate,omitempty"`
	EndDate             string   `json:"endDate,omitempty"`
	NumberOfOccurrences int32    `json:"numberOfOccurrences,omitempty"`
}

// CalendarInfo is a simplified calendar representation
type CalendarInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name" untrusted:"true"`
	Color string `json:"color"`
	Owner string `json:"owner" untrusted:"true"`
}

// ListEvents returns calendar events in [startTime, endTime] from the target
// mailbox, or the signed-in user's calendar when target is empty. Targeting
// another mailbox requires the calling token to carry Calendars.Read.Shared
// and the calling user to have Exchange Full Access delegation on the target.
func (c *Client) ListEvents(ctx context.Context, target string, startTime, endTime time.Time, calendarID string, top int32, preference BodyPreference) ([]CalendarEvent, error) {
	top = clampTop(top)

	headers, options, contract, err := newBodyResponseContract(preference)
	if err != nil {
		return nil, err
	}
	startStr := startTime.UTC().Format("2006-01-02T15:04:05")
	endStr := endTime.UTC().Format("2006-01-02T15:04:05")

	queryParams := &users.ItemCalendarViewRequestBuilderGetQueryParameters{
		StartDateTime: &startStr,
		EndDateTime:   &endStr,
		Top:           &top,
		Select:        []string{"id", "subject", "start", "end", "location", "organizer", "attendees", "isAllDay", "isOnlineMeeting", "onlineMeetingUrl", "showAs", "bodyPreview", "recurrence", "iCalUId", "changeKey", "type", "seriesMasterId", "originalStart", "isCancelled", "isReminderOn", "responseStatus", "transactionId"},
		Orderby:       []string{"start/dateTime"},
	}
	if preference != BodyDefault {
		queryParams.Select = append(queryParams.Select, "body")
	}

	if calendarID != "" {
		if err := validateID(calendarID, "calendar ID"); err != nil {
			return nil, err
		}
		calConfig := &users.ItemCalendarsItemCalendarViewRequestBuilderGetRequestConfiguration{
			Headers: headers, Options: options,
			QueryParameters: &users.ItemCalendarsItemCalendarViewRequestBuilderGetQueryParameters{
				StartDateTime: &startStr,
				EndDateTime:   &endStr,
				Top:           &top,
				Select:        queryParams.Select,
				Orderby:       queryParams.Orderby,
			},
		}
		resp, err := c.targetUser(target).Calendars().ByCalendarId(calendarID).CalendarView().Get(ctx, calConfig)
		if err != nil {
			return nil, fmt.Errorf("listing events: %w", err)
		}
		if err := contract.verify(); err != nil {
			return nil, fmt.Errorf("listing events: %w", err)
		}
		events := make([]CalendarEvent, 0, len(resp.GetValue()))
		for _, e := range resp.GetValue() {
			events = append(events, convertEvent(e))
		}
		if err := verifyCalendarBody(events, preference); err != nil {
			return nil, fmt.Errorf("listing events: %w", err)
		}
		return events, nil
	}

	config := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
		Headers: headers, Options: options,
		QueryParameters: queryParams,
	}

	resp, err := c.targetUser(target).CalendarView().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	if err := contract.verify(); err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}

	events := make([]CalendarEvent, 0, len(resp.GetValue()))
	for _, e := range resp.GetValue() {
		events = append(events, convertEvent(e))
	}
	if err := verifyCalendarBody(events, preference); err != nil {
		return nil, fmt.Errorf("listing events: %w", err)
	}
	return events, nil
}

// GetEvent returns a single event from the target mailbox, or the signed-in
// user's calendar when target is empty. See ListEvents for scope requirements.
func (c *Client) GetEvent(ctx context.Context, target, eventID string, preference BodyPreference) (*CalendarEvent, error) {
	if err := validateID(eventID, "event ID"); err != nil {
		return nil, err
	}
	headers, options, contract, err := newBodyResponseContract(preference)
	if err != nil {
		return nil, err
	}
	event, err := c.targetUser(target).Events().ByEventId(eventID).Get(ctx, &users.ItemEventsEventItemRequestBuilderGetRequestConfiguration{Headers: headers, Options: options})
	if err != nil {
		return nil, fmt.Errorf("getting event: %w", err)
	}
	if err := contract.verify(); err != nil {
		return nil, fmt.Errorf("getting event: %w", err)
	}
	e := convertEvent(event)
	if event.GetBody() != nil && event.GetBody().GetContent() != nil {
		e.Body = *event.GetBody().GetContent()
	}
	if err := verifyCalendarBody([]CalendarEvent{e}, preference); err != nil {
		return nil, fmt.Errorf("getting event: %w", err)
	}
	return &e, nil
}

func verifyCalendarBody(events []CalendarEvent, preference BodyPreference) error {
	if preference == BodyDefault {
		return nil
	}
	for _, event := range events {
		if !strings.EqualFold(event.BodyType, string(preference)) {
			return fmt.Errorf("provider body type = %q, want %q", event.BodyType, preference)
		}
	}
	return nil
}

func (c *Client) CreateEvent(ctx context.Context, calendarID, subject string, start, end time.Time, location string, attendees []string, isAllDay, isOnlineMeeting bool, recurrence, transactionID string, reminderOn *bool) (*CalendarEvent, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	if len(attendees) > 0 {
		if err := c.ensureMaySend(); err != nil {
			return nil, err
		}
	}
	if calendarID != "" {
		if err := validateID(calendarID, "calendar ID"); err != nil {
			return nil, err
		}
	}
	if err := ValidateTransactionID(transactionID); err != nil {
		return nil, err
	}
	event := models.NewEvent()
	event.SetSubject(&subject)

	startDt := models.NewDateTimeTimeZone()
	startStr := start.UTC().Format("2006-01-02T15:04:05")
	startDt.SetDateTime(&startStr)
	utc := graphTimeZoneUTC
	startDt.SetTimeZone(&utc)
	event.SetStart(startDt)

	endDt := models.NewDateTimeTimeZone()
	endStr := end.UTC().Format("2006-01-02T15:04:05")
	endDt.SetDateTime(&endStr)
	endDt.SetTimeZone(&utc)
	event.SetEnd(endDt)

	event.SetIsAllDay(&isAllDay)
	event.SetIsOnlineMeeting(&isOnlineMeeting)
	if reminderOn != nil {
		event.SetIsReminderOn(reminderOn)
	}
	if transactionID != "" {
		event.SetTransactionId(&transactionID)
	}

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

	if recurrence != "" {
		rec, err := buildRecurrence(recurrence, start)
		if err != nil {
			return nil, err
		}
		event.SetRecurrence(rec)
	}

	var created models.Eventable
	var err error
	if calendarID == "" {
		created, err = c.inner.Me().Events().Post(ctx, event, nil)
	} else {
		created, err = c.inner.Me().Calendars().ByCalendarId(calendarID).Events().Post(ctx, event, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("creating event: %w", err)
	}
	e := convertEvent(created)
	return &e, nil
}

func (c *Client) UpdateEvent(ctx context.Context, eventID string, subject *string, start, end *time.Time, location *string, allDay, reminderOn *bool) (*CalendarEvent, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
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
		utc := graphTimeZoneUTC
		startDt.SetTimeZone(&utc)
		event.SetStart(startDt)
	}
	if end != nil {
		endDt := models.NewDateTimeTimeZone()
		endStr := end.UTC().Format("2006-01-02T15:04:05")
		endDt.SetDateTime(&endStr)
		utc := graphTimeZoneUTC
		endDt.SetTimeZone(&utc)
		event.SetEnd(endDt)
	}
	if location != nil {
		loc := models.NewLocation()
		loc.SetDisplayName(location)
		event.SetLocation(loc)
	}
	if allDay != nil {
		event.SetIsAllDay(allDay)
	}
	if reminderOn != nil {
		event.SetIsReminderOn(reminderOn)
	}

	updated, err := c.inner.Me().Events().ByEventId(eventID).Patch(ctx, event, nil)
	if err != nil {
		return nil, fmt.Errorf("updating event: %w", err)
	}
	e := convertEvent(updated)
	return &e, nil
}

func (c *Client) DeleteEvent(ctx context.Context, eventID string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	if err := validateID(eventID, "event ID"); err != nil {
		return err
	}
	err := c.inner.Me().Events().ByEventId(eventID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting event: %w", err)
	}
	return nil
}

func (c *Client) RespondToEvent(ctx context.Context, eventID, response string) error {
	if err := c.ensureMaySend(); err != nil {
		return err
	}
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

// ListCalendars returns calendars from the target mailbox, or the signed-in
// user's mailbox when target is empty. See ListEvents for scope requirements.
func (c *Client) ListCalendars(ctx context.Context, target string) ([]CalendarInfo, error) {
	resp, err := c.targetUser(target).Calendars().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing calendars: %w", err)
	}

	calendars := make([]CalendarInfo, 0, len(resp.GetValue()))
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
		ev.Start = normalizeGraphUTC(*e.GetStart().GetDateTime())
	}
	if e.GetEnd() != nil && e.GetEnd().GetDateTime() != nil {
		ev.End = normalizeGraphUTC(*e.GetEnd().GetDateTime())
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
			d := AttendeeDetails{}
			if a.GetEmailAddress() != nil {
				d.Name = derefStr(a.GetEmailAddress().GetName())
				d.Address = derefStr(a.GetEmailAddress().GetAddress())
			}
			if a.GetTypeEscaped() != nil {
				d.Type = a.GetTypeEscaped().String()
			}
			if a.GetStatus() != nil {
				if a.GetStatus().GetResponse() != nil {
					d.Response = a.GetStatus().GetResponse().String()
				}
				if a.GetStatus().GetTime() != nil {
					d.ResponseTime = a.GetStatus().GetTime().Format(time.RFC3339)
				}
			}
			ev.AttendeeDetails = append(ev.AttendeeDetails, d)
		}
	}
	if s := e.GetResponseStatus(); s != nil {
		ev.ResponseStatus = &EventResponseStatus{}
		if s.GetResponse() != nil {
			ev.ResponseStatus.Response = s.GetResponse().String()
		}
		if s.GetTime() != nil {
			ev.ResponseStatus.Time = s.GetTime().Format(time.RFC3339)
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
	if e.GetBody() != nil {
		if e.GetBody().GetContent() != nil {
			ev.Body = *e.GetBody().GetContent()
		}
		if e.GetBody().GetContentType() != nil {
			ev.BodyType = e.GetBody().GetContentType().String()
		}
	}
	if e.GetICalUId() != nil {
		ev.ICalUID = *e.GetICalUId()
	}
	if e.GetChangeKey() != nil {
		ev.ChangeKey = *e.GetChangeKey()
	}
	if e.GetCreatedDateTime() != nil {
		ev.CreatedAt = e.GetCreatedDateTime().Format(time.RFC3339)
	}
	if e.GetLastModifiedDateTime() != nil {
		ev.ModifiedAt = e.GetLastModifiedDateTime().Format(time.RFC3339)
	}
	if e.GetTypeEscaped() != nil {
		ev.EventType = e.GetTypeEscaped().String()
	}
	if e.GetSeriesMasterId() != nil {
		ev.SeriesMasterID = *e.GetSeriesMasterId()
	}
	if e.GetOriginalStart() != nil {
		ev.OriginalStart = e.GetOriginalStart().Format(time.RFC3339)
	}
	if e.GetIsCancelled() != nil {
		ev.IsCancelled = *e.GetIsCancelled()
	}
	if e.GetIsReminderOn() != nil {
		v := *e.GetIsReminderOn()
		ev.IsReminderOn = &v
	}
	if e.GetTransactionId() != nil {
		ev.TransactionID = *e.GetTransactionId()
	}
	if e.GetRecurrence() != nil {
		ev.Recurrence = formatRecurrence(e.GetRecurrence())
		ev.RecurrenceDetails = convertRecurrenceDetails(e.GetRecurrence())
	}
	return ev
}

func convertRecurrenceDetails(r models.PatternedRecurrenceable) *RecurrenceDetails {
	d := &RecurrenceDetails{}
	if p := r.GetPattern(); p != nil {
		if p.GetTypeEscaped() != nil {
			d.PatternType = p.GetTypeEscaped().String()
		}
		if p.GetInterval() != nil {
			d.Interval = *p.GetInterval()
		}
		for _, day := range p.GetDaysOfWeek() {
			d.DaysOfWeek = append(d.DaysOfWeek, day.String())
		}
		if p.GetDayOfMonth() != nil {
			d.DayOfMonth = *p.GetDayOfMonth()
		}
		if p.GetMonth() != nil {
			d.Month = *p.GetMonth()
		}
		if p.GetIndex() != nil {
			d.Index = p.GetIndex().String()
		}
	}
	if rr := r.GetRangeEscaped(); rr != nil {
		if rr.GetTypeEscaped() != nil {
			d.RangeType = rr.GetTypeEscaped().String()
		}
		if rr.GetStartDate() != nil {
			d.StartDate = rr.GetStartDate().String()
		}
		if rr.GetEndDate() != nil {
			d.EndDate = rr.GetEndDate().String()
		}
		if rr.GetNumberOfOccurrences() != nil {
			d.NumberOfOccurrences = *rr.GetNumberOfOccurrences()
		}
	}
	return d
}

// buildRecurrence creates a PatternedRecurrence from a simple recurrence string.
func buildRecurrence(recurrence string, start time.Time) (models.PatternedRecurrenceable, error) {
	rec := models.NewPatternedRecurrence()
	pattern := models.NewRecurrencePattern()
	interval := int32(1)
	pattern.SetInterval(&interval)

	switch recurrence {
	case "daily":
		patType := models.DAILY_RECURRENCEPATTERNTYPE
		pattern.SetTypeEscaped(&patType)
	case "weekdays":
		patType := models.WEEKLY_RECURRENCEPATTERNTYPE
		pattern.SetTypeEscaped(&patType)
		pattern.SetDaysOfWeek([]models.DayOfWeek{
			models.MONDAY_DAYOFWEEK, models.TUESDAY_DAYOFWEEK,
			models.WEDNESDAY_DAYOFWEEK, models.THURSDAY_DAYOFWEEK,
			models.FRIDAY_DAYOFWEEK,
		})
	case "weekly":
		patType := models.WEEKLY_RECURRENCEPATTERNTYPE
		pattern.SetTypeEscaped(&patType)
		dow := dayOfWeekFromTime(start)
		pattern.SetDaysOfWeek([]models.DayOfWeek{dow})
	case "monthly":
		patType := models.ABSOLUTEMONTHLY_RECURRENCEPATTERNTYPE
		pattern.SetTypeEscaped(&patType)
		day := int32(start.Day())
		pattern.SetDayOfMonth(&day)
	case "yearly":
		patType := models.ABSOLUTEYEARLY_RECURRENCEPATTERNTYPE
		pattern.SetTypeEscaped(&patType)
		day := int32(start.Day())
		pattern.SetDayOfMonth(&day)
		month := int32(start.Month())
		pattern.SetMonth(&month)
	default:
		return nil, fmt.Errorf("invalid recurrence %q: use daily, weekdays, weekly, monthly, or yearly", recurrence)
	}

	rec.SetPattern(pattern)

	// Set range: no end date (recur forever)
	recRange := models.NewRecurrenceRange()
	rangeType := models.NOEND_RECURRENCERANGETYPE
	recRange.SetTypeEscaped(&rangeType)
	startDate := serialization.NewDateOnly(start)
	recRange.SetStartDate(startDate)
	rec.SetRangeEscaped(recRange)

	return rec, nil
}

func dayOfWeekFromTime(t time.Time) models.DayOfWeek {
	switch t.Weekday() {
	case time.Sunday:
		return models.SUNDAY_DAYOFWEEK
	case time.Monday:
		return models.MONDAY_DAYOFWEEK
	case time.Tuesday:
		return models.TUESDAY_DAYOFWEEK
	case time.Wednesday:
		return models.WEDNESDAY_DAYOFWEEK
	case time.Thursday:
		return models.THURSDAY_DAYOFWEEK
	case time.Friday:
		return models.FRIDAY_DAYOFWEEK
	case time.Saturday:
		return models.SATURDAY_DAYOFWEEK
	default:
		// time.Weekday is only Sunday..Saturday; keep default for the compiler.
		return models.SATURDAY_DAYOFWEEK
	}
}

// formatRecurrence converts a recurrence pattern to a human-readable string.
func formatRecurrence(r models.PatternedRecurrenceable) string {
	if r.GetPattern() == nil {
		return ""
	}
	p := r.GetPattern()
	if p.GetTypeEscaped() == nil {
		return ""
	}

	interval := int32(1)
	if p.GetInterval() != nil {
		interval = *p.GetInterval()
	}

	switch *p.GetTypeEscaped() {
	case models.DAILY_RECURRENCEPATTERNTYPE:
		if interval == 1 {
			return "Daily"
		}
		return fmt.Sprintf("Every %d days", interval)
	case models.WEEKLY_RECURRENCEPATTERNTYPE:
		days := formatDaysOfWeek(p.GetDaysOfWeek())
		if interval == 1 {
			return fmt.Sprintf("Weekly on %s", days)
		}
		return fmt.Sprintf("Every %d weeks on %s", interval, days)
	case models.ABSOLUTEMONTHLY_RECURRENCEPATTERNTYPE:
		day := int32(1)
		if p.GetDayOfMonth() != nil {
			day = *p.GetDayOfMonth()
		}
		if interval == 1 {
			return fmt.Sprintf("Monthly on day %d", day)
		}
		return fmt.Sprintf("Every %d months on day %d", interval, day)
	case models.RELATIVEMONTHLY_RECURRENCEPATTERNTYPE:
		days := formatDaysOfWeek(p.GetDaysOfWeek())
		if interval == 1 {
			return fmt.Sprintf("Monthly on %s", days)
		}
		return fmt.Sprintf("Every %d months on %s", interval, days)
	case models.ABSOLUTEYEARLY_RECURRENCEPATTERNTYPE:
		day := int32(1)
		if p.GetDayOfMonth() != nil {
			day = *p.GetDayOfMonth()
		}
		if interval == 1 {
			return fmt.Sprintf("Yearly on day %d", day)
		}
		return fmt.Sprintf("Every %d years on day %d", interval, day)
	case models.RELATIVEYEARLY_RECURRENCEPATTERNTYPE:
		days := formatDaysOfWeek(p.GetDaysOfWeek())
		if interval == 1 {
			return fmt.Sprintf("Yearly on %s", days)
		}
		return fmt.Sprintf("Every %d years on %s", interval, days)
	default:
		return "Recurring"
	}
}

func formatDaysOfWeek(days []models.DayOfWeek) string {
	if len(days) == 0 {
		return ""
	}
	names := make([]string, 0, len(days))
	for _, d := range days {
		names = append(names, dayNameTitle.String(d.String()))
	}
	return strings.Join(names, ", ")
}

// ListCalendarView returns expanded occurrences (including recurring) in a date range.
// ListCalendarView is an alias for ListEvents (which already calls the
// calendarView endpoint and expands recurrences). Kept for caller clarity.
func (c *Client) ListCalendarView(ctx context.Context, target string, startTime, endTime time.Time, calendarID string, top int32, preference BodyPreference) ([]CalendarEvent, error) {
	return c.ListEvents(ctx, target, startTime, endTime, calendarID, top, preference)
}

// MeetingTimeSuggestion represents a suggested meeting time
type MeetingTimeSuggestion struct {
	Start                  string                     `json:"start"`
	End                    string                     `json:"end"`
	Confidence             float64                    `json:"confidence"`
	OrganizerAvailability  string                     `json:"organizerAvailability"`
	AttendeeAvailabilities []AttendeeAvailabilityInfo `json:"attendeeAvailabilities,omitempty"`
}

// AttendeeAvailabilityInfo represents an attendee's availability for a time slot
type AttendeeAvailabilityInfo struct {
	Email        string `json:"email"`
	Availability string `json:"availability"`
}

// FindMeetingTimes finds available meeting times for the given attendees
func (c *Client) FindMeetingTimes(ctx context.Context, attendees []string, start, end time.Time, durationMinutes int32) ([]MeetingTimeSuggestion, error) {
	for _, email := range attendees {
		if err := ValidateEmail(email); err != nil {
			return nil, fmt.Errorf("invalid attendee email: %w", err)
		}
	}

	body := users.NewItemFindMeetingTimesPostRequestBody()

	// Set attendees
	attList := make([]models.AttendeeBaseable, 0, len(attendees))
	for _, email := range attendees {
		att := models.NewAttendeeBase()
		addr := models.NewEmailAddress()
		e := email
		addr.SetAddress(&e)
		att.SetEmailAddress(addr)
		required := models.REQUIRED_ATTENDEETYPE
		att.SetTypeEscaped(&required)
		attList = append(attList, att)
	}
	body.SetAttendees(attList)

	// Set time constraint
	tc := models.NewTimeConstraint()
	slot := models.NewTimeSlot()
	startDt := models.NewDateTimeTimeZone()
	startStr := start.UTC().Format("2006-01-02T15:04:05")
	utc := graphTimeZoneUTC
	startDt.SetDateTime(&startStr)
	startDt.SetTimeZone(&utc)
	slot.SetStart(startDt)

	endDt := models.NewDateTimeTimeZone()
	endStr := end.UTC().Format("2006-01-02T15:04:05")
	endDt.SetDateTime(&endStr)
	endDt.SetTimeZone(&utc)
	slot.SetEnd(endDt)

	tc.SetTimeSlots([]models.TimeSlotable{slot})
	body.SetTimeConstraint(tc)

	// Set duration
	duration := serialization.NewDuration(0, 0, 0, 0, int(durationMinutes), 0, 0)
	body.SetMeetingDuration(duration)

	resp, err := c.inner.Me().FindMeetingTimes().Post(ctx, body, nil)
	if err != nil {
		return nil, enterpriseError("finding meeting times", err)
	}

	// Check if the API returned a reason for no suggestions
	if resp.GetEmptySuggestionsReason() != nil && *resp.GetEmptySuggestionsReason() != "" {
		return nil, fmt.Errorf("no meeting times available: %s", *resp.GetEmptySuggestionsReason())
	}

	suggestions := make([]MeetingTimeSuggestion, 0, len(resp.GetMeetingTimeSuggestions()))
	for _, s := range resp.GetMeetingTimeSuggestions() {
		suggestion := MeetingTimeSuggestion{}
		if s.GetConfidence() != nil {
			suggestion.Confidence = *s.GetConfidence()
		}
		if s.GetOrganizerAvailability() != nil {
			suggestion.OrganizerAvailability = s.GetOrganizerAvailability().String()
		}
		if ts := s.GetMeetingTimeSlot(); ts != nil {
			if ts.GetStart() != nil && ts.GetStart().GetDateTime() != nil {
				suggestion.Start = normalizeGraphUTC(*ts.GetStart().GetDateTime())
			}
			if ts.GetEnd() != nil && ts.GetEnd().GetDateTime() != nil {
				suggestion.End = normalizeGraphUTC(*ts.GetEnd().GetDateTime())
			}
		}
		for _, a := range s.GetAttendeeAvailability() {
			ai := AttendeeAvailabilityInfo{}
			if a.GetAvailability() != nil {
				ai.Availability = a.GetAvailability().String()
			}
			if a.GetAttendee() != nil && a.GetAttendee().GetEmailAddress() != nil && a.GetAttendee().GetEmailAddress().GetAddress() != nil {
				ai.Email = *a.GetAttendee().GetEmailAddress().GetAddress()
			}
			suggestion.AttendeeAvailabilities = append(suggestion.AttendeeAvailabilities, ai)
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions, nil
}
