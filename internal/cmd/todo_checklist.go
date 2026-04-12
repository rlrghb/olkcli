package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// TodoChecklistCmd manages checklist items within a task.
type TodoChecklistCmd struct {
	List   TodoChecklistListCmd   `cmd:"" help:"List checklist items"`
	Create TodoChecklistCreateCmd `cmd:"" help:"Create a checklist item"`
	Toggle TodoChecklistToggleCmd `cmd:"" help:"Toggle a checklist item checked/unchecked"`
	Update TodoChecklistUpdateCmd `cmd:"" help:"Update a checklist item"`
	Delete TodoChecklistDeleteCmd `cmd:"" help:"Delete a checklist item"`
}

// TodoChecklistListCmd lists checklist items in a task.
type TodoChecklistListCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoChecklistListCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	items, err := client.ListChecklistItems(ctx.Ctx, listID, c.TaskID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(items, len(items), "")
	}

	headers := []string{"ID", "NAME", "CHECKED"}
	var rows [][]string
	for _, item := range items {
		checked := " "
		if item.IsChecked {
			checked = "Y"
		}
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(item.ID), 15),
			outfmt.Truncate(outfmt.Sanitize(item.DisplayName), 50),
			checked,
		})
	}

	return printer.Print(headers, rows, items, len(items), "")
}

// TodoChecklistCreateCmd creates a new checklist item.
type TodoChecklistCreateCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	Name   string `help:"Checklist item name" required:"" short:"n"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoChecklistCreateCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create checklist item %q in task %s\n", outfmt.Sanitize(c.Name), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	item, err := client.CreateChecklistItem(ctx.Ctx, listID, c.TaskID, c.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Checklist item created: %s (ID: %s)\n", outfmt.Sanitize(item.DisplayName), outfmt.Sanitize(item.ID))
	return nil
}

// TodoChecklistToggleCmd toggles a checklist item checked/unchecked.
type TodoChecklistToggleCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	ItemID string `arg:"" help:"Checklist item ID"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoChecklistToggleCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would toggle checklist item %s in task %s\n", outfmt.Sanitize(c.ItemID), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	item, err := client.ToggleChecklistItem(ctx.Ctx, listID, c.TaskID, c.ItemID)
	if err != nil {
		return err
	}

	fmt.Printf("Checklist item toggled: %s (checked: %t)\n", outfmt.Sanitize(item.DisplayName), item.IsChecked)
	return nil
}

// TodoChecklistUpdateCmd updates a checklist item.
type TodoChecklistUpdateCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	ItemID string `arg:"" help:"Checklist item ID"`
	Name   string `help:"New name" short:"n"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoChecklistUpdateCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	var name *string
	if c.Name != "" {
		name = &c.Name
	}

	if name == nil {
		return fmt.Errorf("nothing to update; provide at least --name")
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would update checklist item %s in task %s\n", outfmt.Sanitize(c.ItemID), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	item, err := client.UpdateChecklistItem(ctx.Ctx, listID, c.TaskID, c.ItemID, name, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Checklist item updated: %s\n", outfmt.Sanitize(item.DisplayName))
	return nil
}

// TodoChecklistDeleteCmd deletes a checklist item.
type TodoChecklistDeleteCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	ItemID string `arg:"" help:"Checklist item ID"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoChecklistDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete checklist item %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ItemID, 30)))
	}

	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete checklist item %s in task %s\n", outfmt.Sanitize(c.ItemID), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.DeleteChecklistItem(ctx.Ctx, listID, c.TaskID, c.ItemID)
	if err != nil {
		return err
	}

	fmt.Println("Checklist item deleted.")
	return nil
}
