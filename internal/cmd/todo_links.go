package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// TodoLinksCmd manages linked resources on tasks.
type TodoLinksCmd struct {
	List   TodoLinksListCmd   `cmd:"" help:"List linked resources"`
	Create TodoLinksCreateCmd `cmd:"" help:"Create a linked resource"`
	Delete TodoLinksDeleteCmd `cmd:"" help:"Delete a linked resource"`
}

// TodoLinksListCmd lists linked resources for a task.
type TodoLinksListCmd struct {
	TaskID string `arg:"" help:"Task ID"`
	List   string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoLinksListCmd) Run(ctx *RunContext) error {
	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	links, err := client.ListLinkedResources(ctx.Ctx, listID, c.TaskID)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(links, len(links), "")
	}

	headers := []string{"ID", "NAME", "APP", "URL"}
	var rows [][]string
	for _, l := range links {
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(l.ID), 15),
			outfmt.Truncate(outfmt.Sanitize(l.DisplayName), 30),
			outfmt.Sanitize(l.ApplicationName),
			outfmt.Truncate(outfmt.Sanitize(l.WebURL), 50),
		})
	}

	return printer.Print(headers, rows, links, len(links), "")
}

// TodoLinksCreateCmd creates a linked resource on a task.
type TodoLinksCreateCmd struct {
	TaskID     string `arg:"" help:"Task ID"`
	Name       string `help:"Display name" required:"" short:"n"`
	URL        string `help:"Web URL"`
	AppName    string `help:"Application name" default:"olk"`
	ExternalID string `help:"External ID"`
	List       string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoLinksCreateCmd) Run(ctx *RunContext) error {
	if c.URL != "" && !strings.HasPrefix(c.URL, "http://") && !strings.HasPrefix(c.URL, "https://") {
		return fmt.Errorf("URL must use http:// or https:// scheme")
	}

	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create linked resource %q on task %s\n", outfmt.Sanitize(c.Name), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	link, err := client.CreateLinkedResource(ctx.Ctx, listID, c.TaskID, c.Name, c.AppName, c.ExternalID, c.URL)
	if err != nil {
		return err
	}

	fmt.Printf("Linked resource created: %s (ID: %s)\n", outfmt.Sanitize(link.DisplayName), outfmt.Sanitize(link.ID))
	return nil
}

// TodoLinksDeleteCmd deletes a linked resource from a task.
type TodoLinksDeleteCmd struct {
	TaskID     string `arg:"" help:"Task ID"`
	ResourceID string `arg:"" help:"Linked resource ID"`
	List       string `help:"Task list ID" env:"OLK_TODO_LIST"`
}

func (c *TodoLinksDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete linked resource %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ResourceID, 30)))
	}

	listID, err := resolveListID(ctx, c.List)
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete linked resource %s from task %s\n", outfmt.Sanitize(c.ResourceID), outfmt.Sanitize(c.TaskID))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	err = client.DeleteLinkedResource(ctx.Ctx, listID, c.TaskID, c.ResourceID)
	if err != nil {
		return err
	}

	fmt.Println("Linked resource deleted.")
	return nil
}
