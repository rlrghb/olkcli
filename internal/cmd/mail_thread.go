package cmd

import "github.com/rlrghb/olkcli/internal/graphapi"

// MailThreadCmd returns every message in a conversation, oldest first. The
// conversation id comes from a message's conversationId field (shown by mail
// list and mail get).
type MailThreadCmd struct {
	ConversationID string  `arg:"" help:"Conversation ID (a message's conversationId)" name:"conversation-id"`
	Top            int32   `help:"Max messages to return" default:"50" short:"n"`
	Complete       bool    `help:"Consume every provider page before returning"`
	BodyFormat     *string `help:"Request provider-returned message body representation" enum:"text,html"`
}

func (c *MailThreadCmd) Run(ctx *RunContext) error {
	client, err := ctx.GraphClient()
	if err != nil {
		return err
	}
	target, err := resolveMailboxTarget(ctx.Flags.Mailbox)
	if err != nil {
		return err
	}

	bodyFormat := ""
	if c.BodyFormat != nil {
		bodyFormat = *c.BodyFormat
	}
	preference, err := graphapi.ParseMessageBodyPreference(bodyFormat)
	if err != nil {
		return err
	}
	var messages []graphapi.MailMessage
	if c.Complete {
		messages, err = client.ListCompleteThread(
			ctx.Ctx,
			target,
			c.ConversationID,
			preference,
		)
	} else {
		messages, err = client.ListThread(
			ctx.Ctx,
			target,
			c.ConversationID,
			c.Top,
			preference,
		)
	}
	if err != nil {
		return err
	}
	return printMessageList(ctx, messages)
}
