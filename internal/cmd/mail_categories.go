package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailCategoriesCmd manages the master category list
type MailCategoriesCmd struct {
	List   MailCategoriesListCmd   `cmd:"" help:"List available categories"`
	Create MailCategoriesCreateCmd `cmd:"" help:"Create a category"`
	Delete MailCategoriesDeleteCmd `cmd:"" help:"Delete a category"`
}

// MailCategoriesListCmd lists all master categories
type MailCategoriesListCmd struct{}

func (c *MailCategoriesListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	categories, err := client.ListCategories(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(categories, len(categories), "")
	}

	headers := []string{"ID", "NAME", "COLOR"}
	var rows [][]string
	for _, cat := range categories {
		rows = append(rows, []string{
			outfmt.Truncate(outfmt.Sanitize(cat.ID), 20),
			outfmt.Sanitize(cat.DisplayName),
			outfmt.Sanitize(cat.Color),
		})
	}

	return printer.Print(headers, rows, categories, len(categories), "")
}

// MailCategoriesCreateCmd creates a new category
type MailCategoriesCreateCmd struct {
	Name  string `help:"Category name" required:"" short:"n"`
	Color string `name:"preset" help:"Color preset: none, preset0 through preset24" default:""`
}

func (c *MailCategoriesCreateCmd) Run(ctx *RunContext) error {
	if len(c.Name) == 0 {
		return fmt.Errorf("category name cannot be empty")
	}
	if len(c.Name) > 255 {
		return fmt.Errorf("category name too long (max 255 characters)")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create category %q\n", outfmt.Sanitize(c.Name))
		return nil
	}

	cat, err := client.CreateCategory(ctx.Ctx, c.Name, c.Color)
	if err != nil {
		return err
	}

	fmt.Printf("Category created: %s (ID: %s)\n", outfmt.Sanitize(cat.DisplayName), outfmt.Sanitize(cat.ID))
	return nil
}

// MailCategoriesDeleteCmd deletes a category
type MailCategoriesDeleteCmd struct {
	ID string `arg:"" help:"Category ID"`
}

func (c *MailCategoriesDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete category %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ID, 30)))
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete category %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.DeleteCategory(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Category deleted.")
	return nil
}
