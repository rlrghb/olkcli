package cmd

import (
	"fmt"
	"strings"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailRulesCmd struct {
	List   MailRulesListCmd   `cmd:"" help:"List inbox rules"`
	Create MailRulesCreateCmd `cmd:"" help:"Create an inbox rule"`
	Delete MailRulesDeleteCmd `cmd:"" help:"Delete an inbox rule"`
}

type MailRulesListCmd struct{}

func (c *MailRulesListCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	rules, err := client.ListMailRules(ctx.Ctx)
	if err != nil {
		return err
	}

	printer := ctx.Printer()
	if ctx.Flags.JSON {
		return printer.PrintJSON(rules, len(rules), "")
	}

	headers := []string{"ID", "NAME", "ENABLED", "CONDITIONS", "ACTIONS"}
	var rows [][]string
	for _, r := range rules {
		id := outfmt.Truncate(r.ID, 15)
		enabled := "N"
		if r.IsEnabled {
			enabled = "Y"
		}
		rows = append(rows, []string{id, outfmt.Sanitize(r.DisplayName), enabled, outfmt.Sanitize(r.Conditions), outfmt.Sanitize(r.Actions)})
	}

	return printer.Print(headers, rows, rules, len(rules), "")
}

type MailRulesCreateCmd struct {
	Name           string `help:"Rule name" required:"" short:"n"`
	From           string `help:"Match sender email"`
	SubjectContain string `help:"Match subject containing text" name:"subject-contains"`
	HasAttachment  bool   `help:"Match messages with attachments"`
	Move           string `help:"Move to folder ID or well-known name"`
	MarkRead       bool   `help:"Mark as read"`
	Delete         bool   `help:"Delete the message"`
	ForwardTo      string `help:"Forward to email address" name:"forward-to"`
	SetImportance  string `help:"Set importance: low|normal|high" name:"set-importance" enum:",low,normal,high" default:""`
}

func (c *MailRulesCreateCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	// Build conditions
	var conditions []string
	if c.From != "" {
		conditions = append(conditions, fmt.Sprintf("from:%s", c.From))
	}
	if c.SubjectContain != "" {
		conditions = append(conditions, fmt.Sprintf("subject-contains:%s", c.SubjectContain))
	}
	if c.HasAttachment {
		conditions = append(conditions, "has-attachment")
	}
	if len(conditions) == 0 {
		return fmt.Errorf("at least one condition is required (--from, --subject-contains, --has-attachment)")
	}

	// Build actions
	var actions []string
	if c.Move != "" {
		actions = append(actions, fmt.Sprintf("move:%s", c.Move))
	}
	if c.MarkRead {
		actions = append(actions, "mark-read")
	}
	if c.Delete {
		actions = append(actions, "delete")
	}
	if c.ForwardTo != "" {
		actions = append(actions, fmt.Sprintf("forward:%s", c.ForwardTo))
	}
	if c.SetImportance != "" {
		actions = append(actions, fmt.Sprintf("importance:%s", c.SetImportance))
	}
	if len(actions) == 0 {
		return fmt.Errorf("at least one action is required (--move, --mark-read, --delete, --forward-to, --set-importance)")
	}

	if ctx.Flags.DryRun {
		fmt.Printf("Would create rule:\n  Name: %s\n  Conditions: %s\n  Actions: %s\n",
			outfmt.Sanitize(c.Name), outfmt.Sanitize(strings.Join(conditions, ", ")), outfmt.Sanitize(strings.Join(actions, ", ")))
		return nil
	}

	rule, err := client.CreateMailRule(ctx.Ctx, c.Name, c.From, c.SubjectContain, c.HasAttachment, c.Move, c.MarkRead, c.Delete, c.ForwardTo, c.SetImportance)
	if err != nil {
		return err
	}

	fmt.Printf("Rule created: %s (ID: %s)\n", outfmt.Sanitize(rule.DisplayName), rule.ID)
	return nil
}

type MailRulesDeleteCmd struct {
	ID string `arg:"" help:"Rule ID"`
}

func (c *MailRulesDeleteCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if !ctx.Flags.Force {
		return fmt.Errorf("delete rule %s: use --force to confirm deletion", outfmt.Sanitize(c.ID))
	}

	err = client.DeleteMailRule(ctx.Ctx, c.ID)
	if err != nil {
		return err
	}

	fmt.Println("Rule deleted.")
	return nil
}
