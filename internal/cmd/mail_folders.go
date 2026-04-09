package cmd

import "fmt"

type MailFoldersCmd struct{}

func (c *MailFoldersCmd) Run(ctx *RunContext) error {
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
