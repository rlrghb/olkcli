package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

// MailFoldersCmd manages mail folders
type MailFoldersCmd struct {
	List   MailFoldersListCmd   `cmd:"" default:"1" help:"List mail folders"`
	Create MailFoldersCreateCmd `cmd:"" help:"Create a mail folder"`
	Rename MailFoldersRenameCmd `cmd:"" help:"Rename a mail folder"`
	Delete MailFoldersDeleteCmd `cmd:"" help:"Delete a mail folder"`
}

// MailFoldersListCmd lists all mail folders (default subcommand)
type MailFoldersListCmd struct{}

func (c *MailFoldersListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	folders, err := client.ListMailFolders(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(folders, len(folders), "")
	}

	headers := []string{"ID", "NAME", "TOTAL", "UNREAD"}
	var rows [][]string
	for _, f := range folders {
		rows = append(rows, []string{
			f.ID,
			f.DisplayName,
			fmt.Sprintf("%d", f.TotalCount),
			fmt.Sprintf("%d", f.UnreadCount),
		})
	}

	return printer.Print(headers, rows, folders, len(folders), "")
}

// MailFoldersCreateCmd creates a new mail folder
type MailFoldersCreateCmd struct {
	Name string `help:"Folder name" required:"" short:"n"`
}

func (c *MailFoldersCreateCmd) Run(ctx *RunContext) error {
	if len(c.Name) == 0 {
		return fmt.Errorf("folder name cannot be empty")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create folder %q\n", outfmt.Sanitize(c.Name))
		return nil
	}

	folder, err := client.CreateMailFolder(ctx.Ctx, c.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Folder created: %s (ID: %s)\n", outfmt.Sanitize(folder.DisplayName), outfmt.Sanitize(folder.ID))
	return nil
}

// MailFoldersRenameCmd renames a mail folder
type MailFoldersRenameCmd struct {
	ID   string `arg:"" help:"Folder ID"`
	Name string `help:"New folder name" required:"" short:"n"`
}

func (c *MailFoldersRenameCmd) Run(ctx *RunContext) error {
	if len(c.Name) == 0 {
		return fmt.Errorf("folder name cannot be empty")
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would rename folder %s to %q\n", outfmt.Sanitize(c.ID), outfmt.Sanitize(c.Name))
		return nil
	}

	folder, err := client.RenameMailFolder(ctx.Ctx, c.ID, c.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Folder renamed: %s\n", outfmt.Sanitize(folder.DisplayName))
	return nil
}

// MailFoldersDeleteCmd deletes a mail folder
type MailFoldersDeleteCmd struct {
	ID string `arg:"" help:"Folder ID"`
}

func (c *MailFoldersDeleteCmd) Run(ctx *RunContext) error {
	if !ctx.Flags.Force {
		return fmt.Errorf("delete folder %s: use --force to confirm deletion", outfmt.Sanitize(outfmt.Truncate(c.ID, 30)))
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would delete folder %s\n", outfmt.Sanitize(c.ID))
		return nil
	}

	err = client.DeleteMailFolder(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Folder deleted.")
	return nil
}
