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
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Importance  string `json:"importance"`
	DueDate     string `json:"dueDateTime,omitempty"`
	CreatedAt   string `json:"createdDateTime"`
	CompletedAt string `json:"completedDateTime,omitempty"`
	Body        string `json:"body,omitempty"`
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
func (c *Client) CreateTodoTask(ctx context.Context, listID, title string, dueDate string, importance string, body string) (*TodoTask, error) {
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
		tz := "UTC"
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
	return task
}
