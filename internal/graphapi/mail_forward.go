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
	Bcc     []string
	Comment string
	IsHTML  bool
}

type forwardRecipientLists struct {
	to, cc, bcc []models.Recipientable
}

func (r forwardRecipientLists) copied() bool {
	return len(r.cc) > 0 || len(r.bcc) > 0
}

func forwardRecipients(opts *ForwardOptions) (forwardRecipientLists, error) {
	var lists forwardRecipientLists
	if opts == nil {
		return lists, fmt.Errorf("forward options are required")
	}
	var err error
	if lists.to, err = makeRecipients(opts.To); err != nil {
		return lists, fmt.Errorf("invalid forward recipient: %w", err)
	}
	if lists.cc, err = makeRecipients(opts.Cc); err != nil {
		return lists, fmt.Errorf("invalid forward cc recipient: %w", err)
	}
	if lists.bcc, err = makeRecipients(opts.Bcc); err != nil {
		return lists, fmt.Errorf("invalid forward bcc recipient: %w", err)
	}
	return lists, nil
}

// forwardRecipientMessage carries the recipients inside the message payload,
// which is where Graph's documented forward examples put them once a message
// object is present at all.
func forwardRecipientMessage(message models.Messageable, recipients forwardRecipientLists) models.Messageable {
	message.SetToRecipients(recipients.to)
	if len(recipients.cc) > 0 {
		message.SetCcRecipients(recipients.cc)
	}
	if len(recipients.bcc) > 0 {
		message.SetBccRecipients(recipients.bcc)
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
	recipients, err := forwardRecipients(opts)
	if err != nil {
		return err
	}
	comment := opts.Comment
	if !opts.IsHTML {
		comment = plainTextHTML(comment)
	}
	body := users.NewItemMessagesItemForwardPostRequestBody()
	switch {
	case opts.IsHTML:
		body.SetMessage(forwardRecipientMessage(htmlMessageBody(comment), recipients))
	case recipients.copied():
		body.SetComment(&comment)
		body.SetMessage(forwardRecipientMessage(models.NewMessage(), recipients))
	default:
		body.SetComment(&comment)
		body.SetToRecipients(recipients.to)
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
	recipients, err := forwardRecipients(opts)
	if err != nil {
		return nil, err
	}
	htmlOpts := &CreateReplyDraftOptions{Body: opts.Comment, IsHTML: opts.IsHTML}
	if err := validateCreateReplyDraftOptions(htmlOpts); err != nil {
		return nil, err
	}

	body := users.NewItemMessagesItemCreateForwardPostRequestBody()
	body.SetMessage(forwardRecipientMessage(models.NewMessage(), recipients))
	if !opts.IsHTML {
		comment := plainTextHTML(opts.Comment)
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
