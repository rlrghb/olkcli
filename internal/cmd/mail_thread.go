package cmd

// MailThreadCmd returns every message in a conversation, oldest first. The
// conversation id comes from a message's conversationId field (shown by mail
// list and mail get).
type MailThreadCmd struct {
	ConversationID string `arg:"" help:"Conversation ID (a message's conversationId)" name:"conversation-id"`
	Top            int32  `help:"Max messages to return" default:"50" short:"n"`
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

	messages, err := client.ListThread(ctx.Ctx, target, c.ConversationID, c.Top)
	if err != nil {
		return err
	}
	return printMessageList(ctx, messages)
}
