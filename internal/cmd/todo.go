package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// TodoCmd is the top-level command group for Microsoft To Do tasks.
type TodoCmd struct {
	Lists     TodoListsCmd     `cmd:"" help:"Task list operations"`
	List      TodoListCmd      `cmd:"" help:"List tasks in a list"`
	Get       TodoGetCmd       `cmd:"" help:"Get task details"`
	Create    TodoCreateCmd    `cmd:"" help:"Create a task"`
	Complete  TodoCompleteCmd  `cmd:"" help:"Mark a task as complete"`
	Update    TodoUpdateCmd    `cmd:"" help:"Update a task"`
	Delete    TodoDeleteCmd    `cmd:"" help:"Delete a task"`
	Checklist TodoChecklistCmd `cmd:"" help:"Checklist item operations"`
	Attach    TodoAttachCmd    `cmd:"" help:"Task attachment operations"`
	Links     TodoLinksCmd     `cmd:"" help:"Linked resource operations"`
}

// resolveListID returns the provided listID, or auto-detects the default task list.
func resolveListID(ctx *RunContext, listID string) (string, error) {
	if listID != "" {
		return listID, nil
	}
	client, err := ctx.GraphClient()
	if err != nil {
		return "", err
	}
	lists, err := client.ListTodoLists(ctx.Ctx)
	if err != nil {
		return "", fmt.Errorf("auto-detecting task list: %w", err)
	}
	if len(lists) == 0 {
		return "", fmt.Errorf("no task lists found; create one in Microsoft To Do first")
	}
	return lists[0].ID, nil
}

// TodoListsCmd manages task lists.
type TodoListsCmd struct {
	List   TodoListsListCmd   `cmd:"" default:"1" help:"List task lists"`
	Create TodoListsCreateCmd `cmd:"" help:"Create a task list"`
	Delete TodoListsDeleteCmd `cmd:"" help:"Delete a task list"`
}

// TodoListsListCmd lists all task lists (default subcommand).
type TodoListsListCmd struct{}

func (c *TodoListsListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	lists, err := client.ListTodoLists(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(lists, len(lists), "")
	}

	headers := []string{"ID", "NAME", "OWNER"}
	var rows [][]string
	for _, l := range lists {
		owner := " "
		if l.IsOwner {
			owner = "Y"
		}
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(l.ID), 15),
			outfmt.Sanitize(l.DisplayName),
			owner,
		})
	}

	return printer.Print(headers, rows, lists, len(lists), "")
}

// TodoListsCreateCmd creates a new task list.
type TodoListsCreateCmd struct {
	Name string `help:"List name" required:"" short:"n"`
}

func (c *TodoListsCreateCmd) Run(ctx *RunContext) error {
	if c.Name == "" {
		return fmt.Errorf("list name cannot be empty")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create task list %q\n", outfmt.Sanitize(c.Name))
		return nil
	}

	list, err := client.CreateTodoList(ctx.Ctx, c.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Task list created: %s (ID: %s)\n", outfmt.Sanitize(list.DisplayName), outfmt.Sanitize(list.ID))
	return nil
}

// TodoListsDeleteCmd deletes a task list.
type TodoListsDeleteCmd struct {
	ID string `arg:"" help:"Task list ID"`
}

func (c *TodoListsDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete task list %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ID, 30)))
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete task list %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.DeleteTodoList(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Task list deleted.")
	return nil
}

// TodoListCmd lists tasks in a task list.
type TodoListCmd struct {
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
	Top    int32  `help:"Number of tasks to return" default:"25" short:"n"`
	Status string `help:"Filter by status" enum:"notStarted,inProgress,completed,waitingOnOthers,deferred," default:""`
}

func (c *TodoListCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	tasks, err := client.ListTodoTasks(ctx.Ctx, listID, c.Top, c.Status)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(tasks, len(tasks), "")
	}

	headers := []string{"ID", "TITLE", "STATUS", "IMPORTANCE", "DUE"}
	var rows [][]string
	for i := range tasks {
		t := &tasks[i]
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(t.ID), 15),
			outfmt.Truncate(outfmt.Sanitize(t.Title), 50),
			outfmt.Sanitize(t.Status),
			outfmt.Sanitize(t.Importance),
			outfmt.Truncate(outfmt.Sanitize(t.DueDate), 16),
		})
	}

	return printer.Print(headers, rows, tasks, len(tasks), "")
}

// TodoGetCmd gets details of a single task.
type TodoGetCmd struct {
	ID   string `arg:"" help:"Task ID"`
	List string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoGetCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	task, err := client.GetTodoTask(ctx.Ctx, listID, c.ID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(task, 1, "")
	}

	fmt.Printf("ID:          %s\n", outfmt.Sanitize(task.ID))
	fmt.Printf("Title:       %s\n", outfmt.Sanitize(task.Title))
	fmt.Printf("Status:      %s\n", outfmt.Sanitize(task.Status))
	fmt.Printf("Importance:  %s\n", outfmt.Sanitize(task.Importance))
	fmt.Printf("Created:     %s\n", outfmt.Sanitize(task.CreatedAt))
	if task.DueDate != "" {
		fmt.Printf("Due:         %s\n", outfmt.Sanitize(task.DueDate))
	}
	if task.StartDate != "" {
		fmt.Printf("Start:       %s\n", outfmt.Sanitize(task.StartDate))
	}
	if task.IsReminderOn {
		fmt.Printf("Reminder:    %s\n", outfmt.Sanitize(task.ReminderDate))
	}
	if task.Recurrence != "" {
		fmt.Printf("Recurrence:  %s\n", outfmt.Sanitize(task.Recurrence))
	}
	if len(task.Categories) > 0 {
		fmt.Printf("Categories:  %s\n", outfmt.Sanitize(strings.Join(task.Categories, ", ")))
	}
	if task.CompletedAt != "" {
		fmt.Printf("Completed:   %s\n", outfmt.Sanitize(task.CompletedAt))
	}
	if task.HasAttachments {
		fmt.Printf("Attachments: Yes\n")
	}
	if task.Body != "" {
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(outfmt.SanitizeMultiline(task.Body))
	}

	return nil
}

// TodoCreateCmd creates a new task.
type TodoCreateCmd struct {
	List       string   `help:"Task list ID" env:"OLK_TODO_LIST"`
	Title      string   `help:"Task title" required:"" short:"t"`
	Due        string   `help:"Due date (ISO 8601, e.g. 2024-12-31)" short:"d"`
	Start      string   `help:"Start date (ISO 8601)" short:"s"`
	Reminder   string   `help:"Reminder date/time (ISO 8601)"`
	Recurrence string   `help:"Recurrence pattern" enum:"daily,weekdays,weekly,monthly,yearly," default:""`
	Importance string   `help:"Importance level" enum:"low,normal,high," default:""`
	Body       string   `help:"Task body text" short:"b"`
	Categories []string `help:"Categories to assign" short:"c"`
}

func (c *TodoCreateCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create task:\n  Title: %s\n", outfmt.Sanitize(c.Title))
		if c.Due != "" {
			fmt.Printf("  Due: %s\n", outfmt.Sanitize(c.Due))
		}
		if c.Start != "" {
			fmt.Printf("  Start: %s\n", outfmt.Sanitize(c.Start))
		}
		if c.Reminder != "" {
			fmt.Printf("  Reminder: %s\n", outfmt.Sanitize(c.Reminder))
		}
		if c.Recurrence != "" {
			fmt.Printf("  Recurrence: %s\n", outfmt.Sanitize(c.Recurrence))
		}
		if c.Importance != "" {
			fmt.Printf("  Importance: %s\n", outfmt.Sanitize(c.Importance))
		}
		if c.Body != "" {
			fmt.Printf("  Body: %s\n", outfmt.Sanitize(c.Body))
		}
		if len(c.Categories) > 0 {
			fmt.Printf("  Categories: %s\n", outfmt.Sanitize(strings.Join(c.Categories, ", ")))
		}
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	task, err := client.CreateTodoTask(ctx.Ctx, listID, c.Title, c.Due, c.Importance, c.Body, c.Start, c.Reminder, c.Recurrence, c.Categories)
	if err != nil {
		return err
	}

	fmt.Printf("Task created: %s (ID: %s)\n", outfmt.Sanitize(task.Title), outfmt.Sanitize(task.ID))
	return nil
}

// TodoCompleteCmd marks a task as complete.
type TodoCompleteCmd struct {
	ID   string `arg:"" help:"Task ID"`
	List string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoCompleteCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would complete task %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.CompleteTodoTask(ctx.Ctx, listID, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Task completed.")
	return nil
}

// TodoUpdateCmd updates a task's properties.
type TodoUpdateCmd struct {
	ID         string   `arg:"" help:"Task ID"`
	List       string   `help:"Task list ID" env:"OLK_TODO_LIST"`
	Title      string   `help:"New title" short:"t"`
	Due        string   `help:"New due date (ISO 8601, empty string to clear)" short:"d"`
	Start      string   `help:"New start date (ISO 8601, empty string to clear)" short:"s"`
	Reminder   string   `help:"New reminder date/time (ISO 8601, empty string to clear)"`
	Recurrence string   `help:"New recurrence pattern (empty string to clear)" enum:"daily,weekdays,weekly,monthly,yearly," default:""`
	Importance string   `help:"New importance level" enum:"low,normal,high," default:""`
	Body       string   `help:"New body text" short:"b"`
	Categories []string `help:"New categories (use -c none to clear)" short:"c"`
}

func (c *TodoUpdateCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	// Build optional params - only set what was provided
	var title, due, importance, body, start, reminder, recurrence *string
	var categories *[]string
	if c.Title != "" {
		title = &c.Title
	}
	if c.Due != "" {
		due = &c.Due
	}
	if c.Importance != "" {
		importance = &c.Importance
	}
	if c.Body != "" {
		body = &c.Body
	}
	if c.Start != "" {
		start = &c.Start
	}
	if c.Reminder != "" {
		reminder = &c.Reminder
	}
	if c.Recurrence != "" {
		recurrence = &c.Recurrence
	}
	if len(c.Categories) > 0 {
		if len(c.Categories) == 1 && c.Categories[0] == "none" {
			empty := []string{}
			categories = &empty
		} else {
			categories = &c.Categories
		}
	}

	if title == nil && due == nil && importance == nil && body == nil && start == nil && reminder == nil && recurrence == nil && categories == nil {
		return fmt.Errorf("nothing to update; provide at least one of --title, --due, --start, --reminder, --recurrence, --importance, --body, --categories")
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would update task %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	task, err := client.UpdateTodoTask(ctx.Ctx, listID, c.ID, title, due, importance, body, start, reminder, recurrence, categories)
	if err != nil {
		return err
	}

	fmt.Printf("Task updated: %s\n", outfmt.Sanitize(task.Title))
	return nil
}

// TodoDeleteCmd deletes a task.
type TodoDeleteCmd struct {
	ID   string `arg:"" help:"Task ID"`
	List string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete task %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete task %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.DeleteTodoTask(ctx.Ctx, listID, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Task deleted.")
	return nil
}
