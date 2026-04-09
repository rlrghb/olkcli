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
	Subject string   `json:"subject"`
	To      []string `json:"to"`
	Body    string   `json:"body,omitempty"`
	Created string   `json:"createdDateTime"`
}

// ListDrafts lists messages in the Drafts folder
func (c *Client) ListDrafts(ctx context.Context, top int32) ([]DraftMessage, error) {
	top = clampTop(top)

	selectFields := []string{"id", "subject", "toRecipients", "body", "createdDateTime"}
	resp, err := c.inner.Me().MailFolders().ByMailFolderId("drafts").Messages().Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
			Top:    &top,
			Select: selectFields,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing drafts: %w", err)
	}

	var drafts []DraftMessage
	for _, msg := range resp.GetValue() {
		drafts = append(drafts, convertDraft(msg))
	}
	return drafts, nil
}

// CreateDraft creates a draft message without sending it
func (c *Client) CreateDraft(ctx context.Context, subject, body string, to, cc, bcc []string, isHTML bool) (*DraftMessage, error) {
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

	result, err := c.inner.Me().Messages().Post(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("creating draft: %w", err)
	}

	draft := convertDraft(result)
	return &draft, nil
}

// SendDraft sends an existing draft message
func (c *Client) SendDraft(ctx context.Context, draftID string) error {
	if err := validateID(draftID, "draft ID"); err != nil {
		return err
	}
	err := c.inner.Me().Messages().ByMessageId(draftID).Send().Post(ctx, nil)
	if err != nil {
		return fmt.Errorf("sending draft: %w", err)
	}
	return nil
}

// DeleteDraft deletes a draft message
func (c *Client) DeleteDraft(ctx context.Context, draftID string) error {
	if err := validateID(draftID, "draft ID"); err != nil {
		return err
	}
	err := c.inner.Me().Messages().ByMessageId(draftID).Delete(ctx, nil)
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
