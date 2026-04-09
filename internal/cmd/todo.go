package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// TodoCmd is the top-level command group for Microsoft To Do tasks.
type TodoCmd struct {
	Lists    TodoListsCmd    `cmd:"" help:"List task lists"`
	List     TodoListCmd     `cmd:"" help:"List tasks in a list"`
	Get      TodoGetCmd      `cmd:"" help:"Get task details"`
	Create   TodoCreateCmd   `cmd:"" help:"Create a task"`
	Complete TodoCompleteCmd `cmd:"" help:"Mark a task as complete"`
	Delete   TodoDeleteCmd   `cmd:"" help:"Delete a task"`
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

// TodoListsCmd lists all task lists.
type TodoListsCmd struct{}

func (c *TodoListsCmd) Run(ctx *RunContext) error {
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
			outfmt.Truncate(l.ID, 15),
			outfmt.Sanitize(l.DisplayName),
			owner,
		})
	}

	return printer.Print(headers, rows, lists, len(lists), "")
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
	for _, t := range tasks {
		rows = append(rows, []string{
			outfmt.Truncate(t.ID, 15),
			outfmt.Truncate(outfmt.Sanitize(t.Title), 50),
			t.Status,
			t.Importance,
			outfmt.Truncate(t.DueDate, 16),
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
	if task.CompletedAt != "" {
		fmt.Printf("Completed:   %s\n", outfmt.Sanitize(task.CompletedAt))
	}
	if task.Body != "" {
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(outfmt.SanitizeMultiline(task.Body))
	}

	return nil
}

// TodoCreateCmd creates a new task.
type TodoCreateCmd struct {
	List       string `help:"Task list ID" env:"OLK_TODO_LIST"`
	Title      string `help:"Task title" required:"" short:"t"`
	Due        string `help:"Due date (ISO 8601, e.g. 2024-12-31)" short:"d"`
	Importance string `help:"Importance level" enum:"low,normal,high," default:""`
	Body       string `help:"Task body text" short:"b"`
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
		if c.Importance != "" {
			fmt.Printf("  Importance: %s\n", outfmt.Sanitize(c.Importance))
		}
		if c.Body != "" {
			fmt.Printf("  Body: %s\n", outfmt.Sanitize(c.Body))
		}
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	task, err := client.CreateTodoTask(ctx.Ctx, listID, c.Title, c.Due, c.Importance, c.Body)
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
