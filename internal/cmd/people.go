package cmd

import (
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type PeopleCmd struct {
	Search PeopleSearchCmd `cmd:"" help:"Search people directory"`
}

type PeopleSearchCmd struct {
	Query string `arg:"" help:"Search query"`
	Top   int32  `help:"Max results to return" default:"25" short:"n"`
}

func (c *PeopleSearchCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	people, err := client.SearchPeople(ctx.Ctx, c.Query, c.Top)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(people, len(people), "")
	}

	headers := []string{"NAME", "EMAIL", "JOB TITLE", "DEPARTMENT", "COMPANY"}
	var rows [][]string
	for _, p := range people {
		rows = append(rows, []string{
			outfmt.Sanitize(p.DisplayName),
			outfmt.Sanitize(p.Email),
			outfmt.Sanitize(p.JobTitle),
			outfmt.Sanitize(p.Department),
			outfmt.Sanitize(p.Company),
		})
	}

	return printer.Print(headers, rows, people, len(people), "")
}
