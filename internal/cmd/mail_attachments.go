package cmd

import "fmt"

type MailAttachmentsCmd struct {
	ID string `arg:"" help:"Message ID"`
}

func (c *MailAttachmentsCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	attachments, err := client.GetAttachments(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		fmt.Println("No attachments.")
		return nil
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(attachments, len(attachments), "")
	}

	headers := []string{"ID", "NAME", "TYPE", "SIZE"}
	var rows [][]string
	for _, a := range attachments {
		rows = append(rows, []string{
			a.ID,
			a.Name,
			a.ContentType,
			fmt.Sprintf("%d", a.Size),
		})
	}

	return printer.Print(headers, rows, attachments, len(attachments), "")
}
