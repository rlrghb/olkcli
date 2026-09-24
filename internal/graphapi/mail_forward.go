package graphapi

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// ForwardOptions carries the recipients and comment for a forward, whether it
// is sent at once or left as a draft. Comment is HTML when IsHTML is set.
type ForwardOptions struct {
	To      []string
	Cc      []string
	Comment string
	IsHTML  bool
}

func forwardRecipients(opts *ForwardOptions) (to, cc []models.Recipientable, err error) {
	if opts == nil {
		return nil, nil, fmt.Errorf("forward options are required")
	}
	to, err = makeRecipients(opts.To)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid forward recipient: %w", err)
	}
	cc, err = makeRecipients(opts.Cc)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid forward cc recipient: %w", err)
	}
	return to, cc, nil
}

// forwardRecipientMessage carries the recipients inside the message payload,
// which is where Graph's documented forward examples put them once a message
// object is present at all.
func forwardRecipientMessage(message models.Messageable, to, cc []models.Recipientable) models.Messageable {
	message.SetToRecipients(to)
	if len(cc) > 0 {
		message.SetCcRecipients(cc)
	}
	return message
}

// ForwardMessage forwards a message from the target mailbox, or from the
// signed-in user's own mailbox when target is empty, and sends it at once. As
// with ReplyMessage, the target selects both the mailbox the original is read
// from and the sending identity.
func (c *Client) ForwardMessage(ctx context.Context, target, messageID string, opts *ForwardOptions) error {
	if err := c.ensureMaySend(); err != nil {
		return err
	}
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	to, cc, err := forwardRecipients(opts)
	if err != nil {
		return err
	}
	comment := opts.Comment
	body := users.NewItemMessagesItemForwardPostRequestBody()
	switch {
	case opts.IsHTML:
		body.SetMessage(forwardRecipientMessage(htmlMessageBody(comment), to, cc))
	case len(cc) > 0:
		body.SetComment(&comment)
		body.SetMessage(forwardRecipientMessage(models.NewMessage(), to, cc))
	default:
		body.SetComment(&comment)
		body.SetToRecipients(to)
	}

	err = c.targetUser(target).Messages().ByMessageId(messageID).Forward().Post(ctx, body, nil)
	if err != nil {
		if target != "" {
			return sharedMailboxError("forward", target, replyGrantHint, err)
		}
		return fmt.Errorf("forward: %w", err)
	}
	return nil
}

// CreateForwardDraft leaves a forward of a message as a draft in the target
// mailbox, or in the signed-in user's own mailbox when target is empty. Graph
// generates the forwarded original, as Outlook does; an HTML comment is
// inserted ahead of it in the same way as for an HTML reply draft.
func (c *Client) CreateForwardDraft(ctx context.Context, target, messageID string, opts *ForwardOptions) (*DraftMessage, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	to, cc, err := forwardRecipients(opts)
	if err != nil {
		return nil, err
	}
	htmlOpts := &CreateReplyDraftOptions{Body: opts.Comment, IsHTML: opts.IsHTML}
	if err := validateCreateReplyDraftOptions(htmlOpts); err != nil {
		return nil, err
	}

	body := users.NewItemMessagesItemCreateForwardPostRequestBody()
	body.SetMessage(forwardRecipientMessage(models.NewMessage(), to, cc))
	if !opts.IsHTML {
		comment := opts.Comment
		body.SetComment(&comment)
	}
	result, err := c.targetUser(target).Messages().ByMessageId(messageID).CreateForward().Post(ctx, body, nil)
	if err != nil {
		return nil, c.replyDraftError("creating forward draft", target, err)
	}
	if result == nil {
		return nil, fmt.Errorf("creating forward draft: Graph returned no draft")
	}
	draftID := derefStr(result.GetId())
	if draftID == "" {
		return nil, fmt.Errorf("creating forward draft: Graph returned a draft without an ID")
	}
	if !opts.IsHTML {
		draft := convertDraft(result)
		return &draft, nil
	}

	draft, err := c.finishHTMLReplyDraft(ctx, target, messageID, draftID, result, htmlOpts, forwardDraftKind)
	if err != nil {
		return nil, c.cleanupFailedDraft(ctx, target, draftID, forwardDraftKind, err)
	}
	return draft, nil
}
