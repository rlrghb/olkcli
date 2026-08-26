package cmd

import (
	"fmt"

	"github.com/rlrghb/olkcli/internal/graphapi"
	"github.com/rlrghb/olkcli/internal/outfmt"
)

type MailReplyCmd struct {
	ID       string   `arg:"" help:"Message ID to reply to"`
	Body     string   `help:"Reply body" required:"" short:"b"`
	HTML     bool     `help:"Reply body is HTML"`
	ReplyAll bool     `help:"Reply to all recipients" short:"a"`
	Draft    bool     `help:"Create a reply draft instead of sending"`
	Inline   []string `help:"Inline image as CID=PATH (repeatable; HTML drafts only)" placeholder:"CID=PATH"`
}

func (c *MailReplyCmd) Run(ctx *RunContext) error {
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}
	if len(c.Inline) > 0 && (!c.HTML || !c.Draft) {
		return fmt.Errorf("--inline requires both --html and --draft")
	}
	inlineAttachments, err := prepareInlineAttachments(c.Inline, c.Body)
	if err != nil {
		return err
	}

	action := "reply"
	displayAction := "Reply"
	if c.ReplyAll {
		action = "reply-all"
		displayAction = "Reply-all"
	}

	if ctx.Flags.DryRun {
		if c.Draft {
			fmt.Printf("Would create %s draft for message %s in %s\n", action, outfmt.Sanitize(c.ID), describeMailbox(target))
			for _, attachment := range inlineAttachments {
				fmt.Printf("  Inline image: %s=%s (%d bytes)\n",
					outfmt.Sanitize(attachment.ContentID), outfmt.Sanitize(attachment.Name), len(attachment.Content))
			}
			return nil
		}
		fmt.Printf("Would %s to message %s as %s\n", action, outfmt.Sanitize(c.ID), describeMailbox(target))
		return nil
	}

	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}

	if c.Draft {
		draft, err := client.CreateReplyDraft(ctx.Ctx, target, c.ID, &graphapi.CreateReplyDraftOptions{
			Body:              c.Body,
			ReplyAll:          c.ReplyAll,
			IsHTML:            c.HTML,
			InlineAttachments: inlineAttachments,
		})
		if err != nil {
			return err
		}
		fmt.Printf("%s draft created in %s: %s (ID: %s)\n",
			displayAction, describeMailbox(target), outfmt.Sanitize(draft.Subject), outfmt.Sanitize(draft.ID))
		return nil
	}

	if err := client.ReplyMessage(ctx.Ctx, target, c.ID, c.Body, c.ReplyAll, c.HTML); err != nil {
		return err
	}

	if target != "" {
		fmt.Printf("Reply sent from %s.\n", target)
		return nil
	}
	if c.ReplyAll {
		fmt.Println("Reply-all sent.")
	} else {
		fmt.Println("Reply sent.")
	}
	return nil
}
