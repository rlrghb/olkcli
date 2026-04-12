package graphapi

import (
	"context"
	"fmt"
	"time"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format")
}

// TodoList is a simplified task list for output
type TodoList struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	IsOwner     bool   `json:"isOwner"`
}

// TodoTask is a simplified task for output
type TodoTask struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Importance     string   `json:"importance"`
	DueDate        string   `json:"dueDateTime,omitempty"`
	CreatedAt      string   `json:"createdDateTime"`
	CompletedAt    string   `json:"completedDateTime,omitempty"`
	Body           string   `json:"body,omitempty"`
	StartDate      string   `json:"startDateTime,omitempty"`
	IsReminderOn   bool     `json:"isReminderOn"`
	ReminderDate   string   `json:"reminderDateTime,omitempty"`
	Recurrence     string   `json:"recurrence,omitempty"`
	Categories     []string `json:"categories,omitempty"`
	HasAttachments bool     `json:"hasAttachments"`
}

// TodoChecklistItem is a simplified checklist item for output
type TodoChecklistItem struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	IsChecked   bool   `json:"isChecked"`
	CreatedAt   string `json:"createdDateTime,omitempty"`
}

// TodoAttachment is a simplified attachment for output
type TodoAttachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int32  `json:"size"`
}

// TodoLinkedResource is a simplified linked resource for output
type TodoLinkedResource struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	ApplicationName string `json:"applicationName"`
	ExternalID      string `json:"externalId"`
	WebURL          string `json:"webUrl"`
}

// ListTodoLists returns all task lists for the current user.
func (c *Client) ListTodoLists(ctx context.Context) ([]TodoList, error) {
	resp, err := c.inner.Me().Todo().Lists().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing todo lists: %w", err)
	}

	var result []TodoList
	for _, l := range resp.GetValue() {
		tl := TodoList{
			DisplayName: derefStr(l.GetDisplayName()),
		}
		if l.GetId() != nil {
			tl.ID = *l.GetId()
		}
		if l.GetIsOwner() != nil {
			tl.IsOwner = *l.GetIsOwner()
		}
		result = append(result, tl)
	}
	return result, nil
}

// CreateTodoList creates a new task list.
func (c *Client) CreateTodoList(ctx context.Context, displayName string) (*TodoList, error) {
	list := models.NewTodoTaskList()
	list.SetDisplayName(&displayName)

	created, err := c.inner.Me().Todo().Lists().Post(ctx, list, nil)
	if err != nil {
		return nil, fmt.Errorf("creating todo list: %w", err)
	}

	result := TodoList{
		DisplayName: derefStr(created.GetDisplayName()),
	}
	if created.GetId() != nil {
		result.ID = *created.GetId()
	}
	if created.GetIsOwner() != nil {
		result.IsOwner = *created.GetIsOwner()
	}
	return &result, nil
}

// DeleteTodoList deletes a task list.
func (c *Client) DeleteTodoList(ctx context.Context, listID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting todo list: %w", err)
	}
	return nil
}

// ListTodoTasks returns tasks in a given task list.
func (c *Client) ListTodoTasks(ctx context.Context, listID string, top int32, status string) ([]TodoTask, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	top = clampTop(top)

	queryParams := &users.ItemTodoListsItemTasksRequestBuilderGetQueryParameters{
		Top: &top,
	}
	if status != "" {
		// SECURITY: whitelist validation is mandatory here — the status value
		// is interpolated into an OData $filter string. Without this check,
		// arbitrary OData injection would be possible.
		validStatuses := map[string]bool{
			"notStarted": true, "inProgress": true, "completed": true,
			"waitingOnOthers": true, "deferred": true,
		}
		if !validStatuses[status] {
			return nil, fmt.Errorf("invalid status: %q", status)
		}
		filter := fmt.Sprintf("status eq '%s'", status)
		queryParams.Filter = &filter
	}

	config := &users.ItemTodoListsItemTasksRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("listing todo tasks: %w", err)
	}

	var result []TodoTask
	for _, t := range resp.GetValue() {
		result = append(result, convertTodoTask(t))
	}
	return result, nil
}

// GetTodoTask returns a single task by ID.
func (c *Client) GetTodoTask(ctx context.Context, listID, taskID string) (*TodoTask, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	t, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting todo task: %w", err)
	}

	task := convertTodoTask(t)
	return &task, nil
}

// CreateTodoTask creates a new task in the given list.
func (c *Client) CreateTodoTask(ctx context.Context, listID, title, dueDate, importance, body, startDate, reminderDate, recurrence string, categories []string) (*TodoTask, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}

	task := models.NewTodoTask()
	task.SetTitle(&title)

	if dueDate != "" {
		// Validate date format before sending to API
		parsed, err := parseDate(dueDate)
		if err != nil {
			return nil, fmt.Errorf("invalid due date %q: use ISO 8601 format (e.g. 2025-06-15 or 2025-06-15T09:00:00Z): %w", dueDate, err)
		}
		canonical := parsed.UTC().Format("2006-01-02T15:04:05")
		dt := models.NewDateTimeTimeZone()
		dt.SetDateTime(&canonical)
		tz := graphTimeZoneUTC
		dt.SetTimeZone(&tz)
		task.SetDueDateTime(dt)
	}

	if importance != "" {
		var imp models.Importance
		switch importance {
		case "low":
			imp = models.LOW_IMPORTANCE
		case "normal":
			imp = models.NORMAL_IMPORTANCE
		case "high":
			imp = models.HIGH_IMPORTANCE
		default:
			return nil, fmt.Errorf("invalid importance: %q (must be low, normal, or high)", importance)
		}
		task.SetImportance(&imp)
	}

	if body != "" {
		b := models.NewItemBody()
		b.SetContent(&body)
		ct := models.TEXT_BODYTYPE
		b.SetContentType(&ct)
		task.SetBody(b)
	}

	if startDate != "" {
		parsed, err := parseDate(startDate)
		if err != nil {
			return nil, fmt.Errorf("invalid start date %q: use ISO 8601 format (e.g. 2025-06-15 or 2025-06-15T09:00:00Z): %w", startDate, err)
		}
		canonical := parsed.UTC().Format("2006-01-02T15:04:05")
		dt := models.NewDateTimeTimeZone()
		dt.SetDateTime(&canonical)
		tz := graphTimeZoneUTC
		dt.SetTimeZone(&tz)
		task.SetStartDateTime(dt)
	}

	if reminderDate != "" {
		parsed, err := parseDate(reminderDate)
		if err != nil {
			return nil, fmt.Errorf("invalid reminder date %q: use ISO 8601 format (e.g. 2025-06-15 or 2025-06-15T09:00:00Z): %w", reminderDate, err)
		}
		canonical := parsed.UTC().Format("2006-01-02T15:04:05")
		dt := models.NewDateTimeTimeZone()
		dt.SetDateTime(&canonical)
		tz := graphTimeZoneUTC
		dt.SetTimeZone(&tz)
		task.SetReminderDateTime(dt)
		isOn := true
		task.SetIsReminderOn(&isOn)
	}

	if recurrence != "" {
		var start time.Time
		switch {
		case startDate != "":
			start, _ = parseDate(startDate)
		case dueDate != "":
			start, _ = parseDate(dueDate)
		default:
			start = time.Now()
		}
		rec, err := buildRecurrence(recurrence, start)
		if err != nil {
			return nil, err
		}
		task.SetRecurrence(rec)
	}

	if len(categories) > 0 {
		task.SetCategories(categories)
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().Post(ctx, task, nil)
	if err != nil {
		return nil, fmt.Errorf("creating todo task: %w", err)
	}

	result := convertTodoTask(resp)
	return &result, nil
}

// CompleteTodoTask marks a task as completed.
func (c *Client) CompleteTodoTask(ctx context.Context, listID, taskID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return err
	}

	task := models.NewTodoTask()
	status := models.COMPLETED_TASKSTATUS
	task.SetStatus(&status)

	_, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Patch(ctx, task, nil)
	if err != nil {
		return fmt.Errorf("completing todo task: %w", err)
	}
	return nil
}

// UpdateTodoTask updates a task's properties.
func (c *Client) UpdateTodoTask(ctx context.Context, listID, taskID string, title, dueDate, importance, body, startDate, reminderDate, recurrence *string, categories *[]string) (*TodoTask, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	task := models.NewTodoTask()

	if title != nil {
		task.SetTitle(title)
	}

	if dueDate != nil {
		if *dueDate == "" {
			// Clear due date
			task.SetDueDateTime(nil)
		} else {
			parsed, err := parseDate(*dueDate)
			if err != nil {
				return nil, fmt.Errorf("invalid due date %q: use ISO 8601 format (e.g. 2025-06-15): %w", *dueDate, err)
			}
			canonical := parsed.UTC().Format("2006-01-02T15:04:05")
			dt := models.NewDateTimeTimeZone()
			dt.SetDateTime(&canonical)
			tz := graphTimeZoneUTC
			dt.SetTimeZone(&tz)
			task.SetDueDateTime(dt)
		}
	}

	if importance != nil {
		var imp models.Importance
		switch *importance {
		case "low":
			imp = models.LOW_IMPORTANCE
		case "normal":
			imp = models.NORMAL_IMPORTANCE
		case "high":
			imp = models.HIGH_IMPORTANCE
		default:
			return nil, fmt.Errorf("invalid importance: %q (must be low, normal, or high)", *importance)
		}
		task.SetImportance(&imp)
	}

	if body != nil {
		b := models.NewItemBody()
		b.SetContent(body)
		ct := models.TEXT_BODYTYPE
		b.SetContentType(&ct)
		task.SetBody(b)
	}

	if startDate != nil {
		if *startDate == "" {
			task.SetStartDateTime(nil)
		} else {
			parsed, err := parseDate(*startDate)
			if err != nil {
				return nil, fmt.Errorf("invalid start date %q: use ISO 8601 format (e.g. 2025-06-15): %w", *startDate, err)
			}
			canonical := parsed.UTC().Format("2006-01-02T15:04:05")
			dt := models.NewDateTimeTimeZone()
			dt.SetDateTime(&canonical)
			tz := graphTimeZoneUTC
			dt.SetTimeZone(&tz)
			task.SetStartDateTime(dt)
		}
	}

	if reminderDate != nil {
		if *reminderDate == "" {
			task.SetReminderDateTime(nil)
			isOff := false
			task.SetIsReminderOn(&isOff)
		} else {
			parsed, err := parseDate(*reminderDate)
			if err != nil {
				return nil, fmt.Errorf("invalid reminder date %q: use ISO 8601 format (e.g. 2025-06-15): %w", *reminderDate, err)
			}
			canonical := parsed.UTC().Format("2006-01-02T15:04:05")
			dt := models.NewDateTimeTimeZone()
			dt.SetDateTime(&canonical)
			tz := graphTimeZoneUTC
			dt.SetTimeZone(&tz)
			task.SetReminderDateTime(dt)
			isOn := true
			task.SetIsReminderOn(&isOn)
		}
	}

	if recurrence != nil {
		if *recurrence == "" {
			task.SetRecurrence(nil)
		} else {
			var start time.Time
			switch {
			case startDate != nil && *startDate != "":
				start, _ = parseDate(*startDate)
			case dueDate != nil && *dueDate != "":
				start, _ = parseDate(*dueDate)
			default:
				start = time.Now()
			}
			rec, err := buildRecurrence(*recurrence, start)
			if err != nil {
				return nil, err
			}
			task.SetRecurrence(rec)
		}
	}

	if categories != nil {
		task.SetCategories(*categories)
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Patch(ctx, task, nil)
	if err != nil {
		return nil, fmt.Errorf("updating todo task: %w", err)
	}

	result := convertTodoTask(resp)
	return &result, nil
}

// DeleteTodoTask deletes a task.
func (c *Client) DeleteTodoTask(ctx context.Context, listID, taskID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return err
	}

	err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting todo task: %w", err)
	}
	return nil
}

func convertTodoTask(t models.TodoTaskable) TodoTask {
	task := TodoTask{}
	if t.GetId() != nil {
		task.ID = *t.GetId()
	}
	if t.GetTitle() != nil {
		task.Title = *t.GetTitle()
	}
	if t.GetStatus() != nil {
		task.Status = t.GetStatus().String()
	}
	if t.GetImportance() != nil {
		task.Importance = t.GetImportance().String()
	}
	if t.GetCreatedDateTime() != nil {
		task.CreatedAt = t.GetCreatedDateTime().Format("2006-01-02T15:04:05Z")
	}
	if t.GetCompletedDateTime() != nil && t.GetCompletedDateTime().GetDateTime() != nil {
		task.CompletedAt = *t.GetCompletedDateTime().GetDateTime()
	}
	if t.GetDueDateTime() != nil && t.GetDueDateTime().GetDateTime() != nil {
		task.DueDate = *t.GetDueDateTime().GetDateTime()
	}
	if t.GetBody() != nil && t.GetBody().GetContent() != nil {
		task.Body = *t.GetBody().GetContent()
	}
	if t.GetStartDateTime() != nil && t.GetStartDateTime().GetDateTime() != nil {
		task.StartDate = *t.GetStartDateTime().GetDateTime()
	}
	if t.GetIsReminderOn() != nil {
		task.IsReminderOn = *t.GetIsReminderOn()
	}
	if t.GetReminderDateTime() != nil && t.GetReminderDateTime().GetDateTime() != nil {
		task.ReminderDate = *t.GetReminderDateTime().GetDateTime()
	}
	if t.GetRecurrence() != nil {
		task.Recurrence = formatRecurrence(t.GetRecurrence())
	}
	if t.GetCategories() != nil {
		task.Categories = t.GetCategories()
	}
	if t.GetHasAttachments() != nil {
		task.HasAttachments = *t.GetHasAttachments()
	}
	return task
}

// --- Checklist Item methods ---

// ListChecklistItems returns all checklist items for a task.
func (c *Client) ListChecklistItems(ctx context.Context, listID, taskID string) ([]TodoChecklistItem, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing checklist items: %w", err)
	}

	var result []TodoChecklistItem
	for _, item := range resp.GetValue() {
		result = append(result, convertChecklistItem(item))
	}
	return result, nil
}

// CreateChecklistItem creates a new checklist item on a task.
func (c *Client) CreateChecklistItem(ctx context.Context, listID, taskID, displayName string) (*TodoChecklistItem, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	item := models.NewChecklistItem()
	item.SetDisplayName(&displayName)

	created, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().Post(ctx, item, nil)
	if err != nil {
		return nil, fmt.Errorf("creating checklist item: %w", err)
	}

	result := convertChecklistItem(created)
	return &result, nil
}

// UpdateChecklistItem updates a checklist item's properties.
func (c *Client) UpdateChecklistItem(ctx context.Context, listID, taskID, itemID string, displayName *string, isChecked *bool) (*TodoChecklistItem, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "checklist item ID"); err != nil {
		return nil, err
	}

	item := models.NewChecklistItem()
	if displayName != nil {
		item.SetDisplayName(displayName)
	}
	if isChecked != nil {
		item.SetIsChecked(isChecked)
	}

	updated, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().ByChecklistItemId(itemID).Patch(ctx, item, nil)
	if err != nil {
		return nil, fmt.Errorf("updating checklist item: %w", err)
	}

	result := convertChecklistItem(updated)
	return &result, nil
}

// DeleteChecklistItem deletes a checklist item.
func (c *Client) DeleteChecklistItem(ctx context.Context, listID, taskID, itemID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return err
	}
	if err := validateID(itemID, "checklist item ID"); err != nil {
		return err
	}

	err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().ByChecklistItemId(itemID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting checklist item: %w", err)
	}
	return nil
}

// ToggleChecklistItem toggles the IsChecked state of a checklist item.
func (c *Client) ToggleChecklistItem(ctx context.Context, listID, taskID, itemID string) (*TodoChecklistItem, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}
	if err := validateID(itemID, "checklist item ID"); err != nil {
		return nil, err
	}

	// Get current state
	current, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().ByChecklistItemId(itemID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting checklist item: %w", err)
	}

	// Flip the IsChecked value
	newChecked := current.GetIsChecked() == nil || !*current.GetIsChecked()

	patch := models.NewChecklistItem()
	patch.SetIsChecked(&newChecked)

	updated, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).ChecklistItems().ByChecklistItemId(itemID).Patch(ctx, patch, nil)
	if err != nil {
		return nil, fmt.Errorf("toggling checklist item: %w", err)
	}

	result := convertChecklistItem(updated)
	return &result, nil
}

func convertChecklistItem(item models.ChecklistItemable) TodoChecklistItem {
	ci := TodoChecklistItem{}
	if item.GetId() != nil {
		ci.ID = *item.GetId()
	}
	if item.GetDisplayName() != nil {
		ci.DisplayName = *item.GetDisplayName()
	}
	if item.GetIsChecked() != nil {
		ci.IsChecked = *item.GetIsChecked()
	}
	if item.GetCreatedDateTime() != nil {
		ci.CreatedAt = item.GetCreatedDateTime().Format("2006-01-02T15:04:05Z")
	}
	return ci
}

// --- Attachment methods ---

// ListTodoAttachments returns all attachments for a task.
func (c *Client) ListTodoAttachments(ctx context.Context, listID, taskID string) ([]TodoAttachment, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Attachments().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing todo attachments: %w", err)
	}

	var result []TodoAttachment
	for _, a := range resp.GetValue() {
		att := TodoAttachment{}
		if a.GetId() != nil {
			att.ID = *a.GetId()
		}
		if a.GetName() != nil {
			att.Name = *a.GetName()
		}
		if a.GetContentType() != nil {
			att.ContentType = *a.GetContentType()
		}
		if a.GetSize() != nil {
			att.Size = *a.GetSize()
		}
		result = append(result, att)
	}
	return result, nil
}

// UploadTodoAttachment uploads a file attachment to a task.
func (c *Client) UploadTodoAttachment(ctx context.Context, listID, taskID, name, contentType string, content []byte) (*TodoAttachment, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	att := models.NewTaskFileAttachment()
	odataType := "#microsoft.graph.taskFileAttachment"
	att.SetOdataType(&odataType)
	att.SetName(&name)
	att.SetContentType(&contentType)
	att.SetContentBytes(content)

	created, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Attachments().Post(ctx, att, nil)
	if err != nil {
		return nil, fmt.Errorf("uploading todo attachment: %w", err)
	}

	result := TodoAttachment{}
	if created.GetId() != nil {
		result.ID = *created.GetId()
	}
	if created.GetName() != nil {
		result.Name = *created.GetName()
	}
	if created.GetContentType() != nil {
		result.ContentType = *created.GetContentType()
	}
	if created.GetSize() != nil {
		result.Size = *created.GetSize()
	}
	return &result, nil
}

// DownloadTodoAttachment downloads a task attachment's content.
func (c *Client) DownloadTodoAttachment(ctx context.Context, listID, taskID, attachmentID string) (name, contentType string, content []byte, err error) {
	if e := validateID(listID, "list ID"); e != nil {
		return "", "", nil, e
	}
	if e := validateID(taskID, "task ID"); e != nil {
		return "", "", nil, e
	}
	if e := validateID(attachmentID, "attachment ID"); e != nil {
		return "", "", nil, e
	}

	att, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Attachments().ByAttachmentBaseId(attachmentID).Get(ctx, nil)
	if err != nil {
		return "", "", nil, fmt.Errorf("downloading todo attachment: %w", err)
	}

	if att.GetName() != nil {
		name = *att.GetName()
	}
	if att.GetContentType() != nil {
		contentType = *att.GetContentType()
	}

	// Type-assert to TaskFileAttachmentable to access ContentBytes
	if fileAtt, ok := att.(models.TaskFileAttachmentable); ok {
		content = fileAtt.GetContentBytes()
	}

	return name, contentType, content, nil
}

// DeleteTodoAttachment deletes a task attachment.
func (c *Client) DeleteTodoAttachment(ctx context.Context, listID, taskID, attachmentID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return err
	}
	if err := validateID(attachmentID, "attachment ID"); err != nil {
		return err
	}

	err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).Attachments().ByAttachmentBaseId(attachmentID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting todo attachment: %w", err)
	}
	return nil
}

// --- Linked Resource methods ---

// ListLinkedResources returns all linked resources for a task.
func (c *Client) ListLinkedResources(ctx context.Context, listID, taskID string) ([]TodoLinkedResource, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	resp, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).LinkedResources().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing linked resources: %w", err)
	}

	var result []TodoLinkedResource
	for _, r := range resp.GetValue() {
		result = append(result, convertLinkedResource(r))
	}
	return result, nil
}

// CreateLinkedResource creates a new linked resource on a task.
func (c *Client) CreateLinkedResource(ctx context.Context, listID, taskID, displayName, appName, externalID, webURL string) (*TodoLinkedResource, error) {
	if err := validateID(listID, "list ID"); err != nil {
		return nil, err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return nil, err
	}

	lr := models.NewLinkedResource()
	lr.SetDisplayName(&displayName)
	lr.SetApplicationName(&appName)
	lr.SetExternalId(&externalID)
	lr.SetWebUrl(&webURL)

	created, err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).LinkedResources().Post(ctx, lr, nil)
	if err != nil {
		return nil, fmt.Errorf("creating linked resource: %w", err)
	}

	result := convertLinkedResource(created)
	return &result, nil
}

// DeleteLinkedResource deletes a linked resource from a task.
func (c *Client) DeleteLinkedResource(ctx context.Context, listID, taskID, resourceID string) error {
	if err := validateID(listID, "list ID"); err != nil {
		return err
	}
	if err := validateID(taskID, "task ID"); err != nil {
		return err
	}
	if err := validateID(resourceID, "linked resource ID"); err != nil {
		return err
	}

	err := c.inner.Me().Todo().Lists().ByTodoTaskListId(listID).Tasks().ByTodoTaskId(taskID).LinkedResources().ByLinkedResourceId(resourceID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting linked resource: %w", err)
	}
	return nil
}

func convertLinkedResource(r models.LinkedResourceable) TodoLinkedResource {
	lr := TodoLinkedResource{}
	if r.GetId() != nil {
		lr.ID = *r.GetId()
	}
	if r.GetDisplayName() != nil {
		lr.DisplayName = *r.GetDisplayName()
	}
	if r.GetApplicationName() != nil {
		lr.ApplicationName = *r.GetApplicationName()
	}
	if r.GetExternalId() != nil {
		lr.ExternalID = *r.GetExternalId()
	}
	if r.GetWebUrl() != nil {
		lr.WebURL = *r.GetWebUrl()
	}
	return lr
}
