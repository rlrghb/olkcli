package graphapi

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// DraftMessage is a simplified draft message for output
type DraftMessage struct {
	ID      string   `json:"id"`
	Subject string   `json:"subject" untrusted:"true"`
	To      []string `json:"to" untrusted:"true"`
	Body    string   `json:"body,omitempty" untrusted:"true"`
	Created string   `json:"createdDateTime"`
}

// ListDrafts lists messages in the Drafts folder of the target mailbox, or of
// the signed-in user's own mailbox when target is empty.
func (c *Client) ListDrafts(ctx context.Context, target string, top int32) ([]DraftMessage, error) {
	top = clampTop(top)

	selectFields := []string{"id", "subject", "toRecipients", "body", "createdDateTime"}
	resp, err := c.targetUser(target).MailFolders().ByMailFolderId("drafts").Messages().Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
			Top:    &top,
			Select: selectFields,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing drafts: %w", err)
	}

	drafts := make([]DraftMessage, 0, len(resp.GetValue()))
	for _, msg := range resp.GetValue() {
		drafts = append(drafts, convertDraft(msg))
	}
	return drafts, nil
}

// CreateDraft leaves a draft in the target mailbox, or in the signed-in user's
// own mailbox when target is empty.
//
// Drafting into a shared mailbox is the lower-privilege alternative to sending
// as one: it needs the Mail.ReadWrite.Shared scope and Full Access on the
// mailbox, but *not* Send As or Send on Behalf Of. The draft lands in that
// mailbox's Drafts folder for a person who does hold those rights to review and
// send, which keeps a human at the point where the message actually leaves.
func (c *Client) CreateDraft(ctx context.Context, target, subject, body string, to, cc, bcc []string, isHTML bool) (*DraftMessage, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	msg := models.NewMessage()
	msg.SetSubject(&subject)

	bodyObj := models.NewItemBody()
	bodyObj.SetContent(&body)
	if isHTML {
		html := models.HTML_BODYTYPE
		bodyObj.SetContentType(&html)
	} else {
		text := models.TEXT_BODYTYPE
		bodyObj.SetContentType(&text)
	}
	msg.SetBody(bodyObj)

	toR, err := makeRecipients(to)
	if err != nil {
		return nil, fmt.Errorf("invalid to recipient: %w", err)
	}
	msg.SetToRecipients(toR)

	if len(cc) > 0 {
		ccR, err := makeRecipients(cc)
		if err != nil {
			return nil, fmt.Errorf("invalid cc recipient: %w", err)
		}
		msg.SetCcRecipients(ccR)
	}
	if len(bcc) > 0 {
		bccR, err := makeRecipients(bcc)
		if err != nil {
			return nil, fmt.Errorf("invalid bcc recipient: %w", err)
		}
		msg.SetBccRecipients(bccR)
	}

	result, err := c.targetUser(target).Messages().Post(ctx, msg, nil)
	if err != nil {
		if target != "" {
			return nil, fmt.Errorf("creating draft in %s: %w\n\nDrafting into another mailbox needs the Mail.ReadWrite.Shared scope (sign in again with --scope Mail.ReadWrite.Shared) and Full Access on that mailbox in Exchange. Read-only access to a mailbox is not enough to leave a draft in it", target, err)
		}
		return nil, fmt.Errorf("creating draft: %w", err)
	}

	draft := convertDraft(result)
	return &draft, nil
}

// SendDraft sends an existing draft from the target mailbox, or from the
// signed-in user's own mailbox when target is empty. Sending a draft that lives
// in someone else's mailbox still needs Send As or Send on Behalf Of — creating
// the draft does not confer the right to send it.
func (c *Client) SendDraft(ctx context.Context, target, draftID string) error {
	if err := c.ensureMaySend(); err != nil {
		return err
	}
	if err := validateID(draftID, "draft ID"); err != nil {
		return err
	}
	err := c.targetUser(target).Messages().ByMessageId(draftID).Send().Post(ctx, nil)
	if err != nil {
		return fmt.Errorf("sending draft: %w", err)
	}
	return nil
}

// DeleteDraft deletes a draft from the target mailbox, or from the signed-in
// user's own mailbox when target is empty.
func (c *Client) DeleteDraft(ctx context.Context, target, draftID string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	if err := validateID(draftID, "draft ID"); err != nil {
		return err
	}
	err := c.targetUser(target).Messages().ByMessageId(draftID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting draft: %w", err)
	}
	return nil
}

// convertDraft converts a Graph API message to a DraftMessage
func convertDraft(msg models.Messageable) DraftMessage {
	d := DraftMessage{}
	if msg.GetId() != nil {
		d.ID = *msg.GetId()
	}
	if msg.GetSubject() != nil {
		d.Subject = *msg.GetSubject()
	}
	for _, r := range msg.GetToRecipients() {
		if r.GetEmailAddress() != nil && r.GetEmailAddress().GetAddress() != nil {
			d.To = append(d.To, *r.GetEmailAddress().GetAddress())
		}
	}
	if msg.GetBody() != nil && msg.GetBody().GetContent() != nil {
		d.Body = *msg.GetBody().GetContent()
	}
	if msg.GetCreatedDateTime() != nil {
		d.Created = msg.GetCreatedDateTime().Format("2006-01-02T15:04:05Z")
	}
	return d
}
