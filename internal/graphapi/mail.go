package graphapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// allowedOrderBy is the set of valid $orderby field values.
var allowedOrderBy = map[string]bool{
	"receivedDateTime desc":          true,
	"receivedDateTime asc":           true,
	"receivedDateTime":               true,
	"subject desc":                   true,
	"subject asc":                    true,
	"subject":                        true,
	"from/emailAddress/address desc": true,
	"from/emailAddress/address asc":  true,
	"from/emailAddress/address":      true,
}

// safeEmailPattern validates basic email format.
var safeEmailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// allowedSelectFields is the set of valid $select field names for messages.
var allowedSelectFields = map[string]bool{
	"id": true, "subject": true, "from": true, "toRecipients": true,
	"ccRecipients": true, "bccRecipients": true, "receivedDateTime": true,
	"isRead": true, "hasAttachments": true, "bodyPreview": true, "body": true,
	"importance": true, "conversationId": true, "parentFolderId": true,
	"sender": true, "replyTo": true, "flag": true, "categories": true,
	"internetMessageId": true, "createdDateTime": true, "lastModifiedDateTime": true,
}

// MailMessage is a simplified mail message for output
type MailMessage struct {
	ID             string   `json:"id"`
	Subject        string   `json:"subject"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	ReceivedAt     string   `json:"receivedDateTime"`
	IsRead         bool     `json:"isRead"`
	HasAttachments bool     `json:"hasAttachments"`
	BodyPreview    string   `json:"bodyPreview"`
	Body           string   `json:"body,omitempty"`
	BodyType       string   `json:"bodyType,omitempty"`
	Categories     []string `json:"categories,omitempty"`
}

// MailFolder is a simplified folder representation
type MailFolder struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	TotalCount     int32  `json:"totalItemCount"`
	UnreadCount    int32  `json:"unreadItemCount"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
}

// ListMessagesOptions for filtering messages
type ListMessagesOptions struct {
	FolderID string
	Top      int32
	Filter   string
	OrderBy  string
	Search   string
	Select   []string
}

func (c *Client) ListMessages(ctx context.Context, opts *ListMessagesOptions) ([]MailMessage, error) {
	if opts == nil {
		opts = &ListMessagesOptions{}
	}
	opts.Top = clampTop(opts.Top)

	if opts.OrderBy == "" {
		opts.OrderBy = "receivedDateTime desc"
	}
	if !allowedOrderBy[opts.OrderBy] {
		return nil, fmt.Errorf("invalid orderBy value: %q", opts.OrderBy)
	}

	var config *users.ItemMessagesRequestBuilderGetRequestConfiguration

	top := opts.Top
	orderBy := opts.OrderBy

	queryParams := &users.ItemMessagesRequestBuilderGetQueryParameters{
		Top: &top,
	}
	// Microsoft Graph does not support $orderBy combined with $search or inferenceClassification filter.
	skipOrderBy := opts.Search != "" || strings.Contains(opts.Filter, "inferenceClassification")
	if !skipOrderBy {
		queryParams.Orderby = []string{orderBy}
	}
	if opts.Filter != "" {
		queryParams.Filter = &opts.Filter
	}
	if opts.Search != "" {
		queryParams.Search = &opts.Search
	}
	if len(opts.Select) > 0 {
		for _, f := range opts.Select {
			if !allowedSelectFields[f] {
				return nil, fmt.Errorf("invalid select field: %q", f)
			}
		}
		queryParams.Select = opts.Select
	} else {
		queryParams.Select = []string{"id", "subject", "from", "toRecipients", "receivedDateTime", "isRead", "hasAttachments", "bodyPreview", "categories"}
	}

	config = &users.ItemMessagesRequestBuilderGetRequestConfiguration{
		QueryParameters: queryParams,
	}

	var result []MailMessage

	if opts.FolderID != "" {
		if err := validateID(opts.FolderID, "folder ID"); err != nil {
			return nil, err
		}
		folderQueryParams := &users.ItemMailFoldersItemMessagesRequestBuilderGetQueryParameters{
			Top:    &top,
			Select: queryParams.Select,
		}
		if !skipOrderBy {
			folderQueryParams.Orderby = []string{orderBy}
		}
		if opts.Filter != "" {
			folderQueryParams.Filter = &opts.Filter
		}
		if opts.Search != "" {
			folderQueryParams.Search = &opts.Search
		}
		resp, err := c.inner.Me().MailFolders().ByMailFolderId(opts.FolderID).Messages().Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
			QueryParameters: folderQueryParams,
		})
		if err != nil {
			return nil, fmt.Errorf("listing messages: %w", err)
		}
		for _, msg := range resp.GetValue() {
			result = append(result, convertMessage(msg))
		}
		return result, nil
	}

	resp, err := c.inner.Me().Messages().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	for _, msg := range resp.GetValue() {
		result = append(result, convertMessage(msg))
	}
	return result, nil
}

func (c *Client) GetMessage(ctx context.Context, messageID string) (*MailMessage, error) {
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	msg, err := c.inner.Me().Messages().ByMessageId(messageID).Get(ctx, &users.ItemMessagesMessageItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMessagesMessageItemRequestBuilderGetQueryParameters{
			Select: []string{"id", "subject", "from", "toRecipients", "ccRecipients", "bccRecipients", "receivedDateTime", "isRead", "hasAttachments", "body", "bodyPreview"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting message: %w", err)
	}
	m := convertMessage(msg)
	if msg.GetBody() != nil {
		content := msg.GetBody().GetContent()
		if content != nil {
			m.Body = *content
		}
		ct := msg.GetBody().GetContentType()
		if ct != nil {
			m.BodyType = ct.String()
		}
	}
	return &m, nil
}

func (c *Client) SendMessage(ctx context.Context, subject, body string, toRecipients, ccRecipients, bccRecipients []string, isHTML bool, attachments []AttachmentInput, importance string, readReceipt bool) error {
	msg := models.NewMessage()
	msg.SetSubject(&subject)

	if readReceipt {
		msg.SetIsReadReceiptRequested(&readReceipt)
	}

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

	toR, err := makeRecipients(toRecipients)
	if err != nil {
		return fmt.Errorf("invalid to recipient: %w", err)
	}
	msg.SetToRecipients(toR)
	if len(ccRecipients) > 0 {
		ccR, err := makeRecipients(ccRecipients)
		if err != nil {
			return fmt.Errorf("invalid cc recipient: %w", err)
		}
		msg.SetCcRecipients(ccR)
	}
	if len(bccRecipients) > 0 {
		bccR, err := makeRecipients(bccRecipients)
		if err != nil {
			return fmt.Errorf("invalid bcc recipient: %w", err)
		}
		msg.SetBccRecipients(bccR)
	}

	if len(attachments) > 0 {
		var atts []models.Attachmentable
		for _, a := range attachments {
			fileAtt := models.NewFileAttachment()
			odataType := "#microsoft.graph.fileAttachment"
			fileAtt.SetOdataType(&odataType)
			name := a.Name
			fileAtt.SetName(&name)
			ct := a.ContentType
			fileAtt.SetContentType(&ct)
			fileAtt.SetContentBytes(a.Content)
			atts = append(atts, fileAtt)
		}
		msg.SetAttachments(atts)
	}

	if importance != "" {
		var imp models.Importance
		switch importance {
		case importanceLow:
			imp = models.LOW_IMPORTANCE
		case importanceNormal:
			imp = models.NORMAL_IMPORTANCE
		case importanceHigh:
			imp = models.HIGH_IMPORTANCE
		default:
			return fmt.Errorf("invalid importance: %q (must be low, normal, or high)", importance)
		}
		msg.SetImportance(&imp)
	}

	sendBody := users.NewItemSendMailPostRequestBody()
	sendBody.SetMessage(msg)
	saveToSent := true
	sendBody.SetSaveToSentItems(&saveToSent)

	if err := c.inner.Me().SendMail().Post(ctx, sendBody, nil); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}

func (c *Client) ReplyMessage(ctx context.Context, messageID, comment string, replyAll bool) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	if replyAll {
		body := users.NewItemMessagesItemReplyAllPostRequestBody()
		body.SetComment(&comment)
		err := c.inner.Me().Messages().ByMessageId(messageID).ReplyAll().Post(ctx, body, nil)
		if err != nil {
			return fmt.Errorf("reply all: %w", err)
		}
		return nil
	}

	body := users.NewItemMessagesItemReplyPostRequestBody()
	body.SetComment(&comment)
	err := c.inner.Me().Messages().ByMessageId(messageID).Reply().Post(ctx, body, nil)
	if err != nil {
		return fmt.Errorf("reply: %w", err)
	}
	return nil
}

func (c *Client) ForwardMessage(ctx context.Context, messageID, comment string, toRecipients []string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	body := users.NewItemMessagesItemForwardPostRequestBody()
	body.SetComment(&comment)
	fwdR, err := makeRecipients(toRecipients)
	if err != nil {
		return fmt.Errorf("invalid forward recipient: %w", err)
	}
	body.SetToRecipients(fwdR)

	err = c.inner.Me().Messages().ByMessageId(messageID).Forward().Post(ctx, body, nil)
	if err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	return nil
}

func (c *Client) MoveMessage(ctx context.Context, messageID, folderID string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	if err := validateID(folderID, "folder ID"); err != nil {
		return err
	}
	body := users.NewItemMessagesItemMovePostRequestBody()
	body.SetDestinationId(&folderID)

	_, err := c.inner.Me().Messages().ByMessageId(messageID).Move().Post(ctx, body, nil)
	if err != nil {
		return fmt.Errorf("move message: %w", err)
	}
	return nil
}

func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	err := c.inner.Me().Messages().ByMessageId(messageID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}

func (c *Client) MarkMessage(ctx context.Context, messageID string, isRead bool) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}
	msg := models.NewMessage()
	msg.SetIsRead(&isRead)

	_, err := c.inner.Me().Messages().ByMessageId(messageID).Patch(ctx, msg, nil)
	if err != nil {
		return fmt.Errorf("updating message: %w", err)
	}
	return nil
}

func (c *Client) ListMailFolders(ctx context.Context) ([]MailFolder, error) {
	var top int32 = 100
	resp, err := c.inner.Me().MailFolders().Get(ctx, &users.ItemMailFoldersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersRequestBuilderGetQueryParameters{
			Top: &top,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}

	var folders []MailFolder
	for _, f := range resp.GetValue() {
		folder := MailFolder{
			DisplayName: derefStr(f.GetDisplayName()),
		}
		if f.GetId() != nil {
			folder.ID = *f.GetId()
		}
		if f.GetTotalItemCount() != nil {
			folder.TotalCount = *f.GetTotalItemCount()
		}
		if f.GetUnreadItemCount() != nil {
			folder.UnreadCount = *f.GetUnreadItemCount()
		}
		if f.GetParentFolderId() != nil {
			folder.ParentFolderID = *f.GetParentFolderId()
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// CreateMailFolder creates a new mail folder.
func (c *Client) CreateMailFolder(ctx context.Context, displayName string) (*MailFolder, error) {
	folder := models.NewMailFolder()
	folder.SetDisplayName(&displayName)

	created, err := c.inner.Me().MailFolders().Post(ctx, folder, nil)
	if err != nil {
		return nil, fmt.Errorf("creating mail folder: %w", err)
	}

	result := MailFolder{
		DisplayName: derefStr(created.GetDisplayName()),
	}
	if created.GetId() != nil {
		result.ID = *created.GetId()
	}
	if created.GetTotalItemCount() != nil {
		result.TotalCount = *created.GetTotalItemCount()
	}
	if created.GetUnreadItemCount() != nil {
		result.UnreadCount = *created.GetUnreadItemCount()
	}
	return &result, nil
}

// RenameMailFolder renames a mail folder.
func (c *Client) RenameMailFolder(ctx context.Context, folderID, displayName string) (*MailFolder, error) {
	if err := validateID(folderID, "folder ID"); err != nil {
		return nil, err
	}

	folder := models.NewMailFolder()
	folder.SetDisplayName(&displayName)

	updated, err := c.inner.Me().MailFolders().ByMailFolderId(folderID).Patch(ctx, folder, nil)
	if err != nil {
		return nil, fmt.Errorf("renaming mail folder: %w", err)
	}

	result := MailFolder{
		DisplayName: derefStr(updated.GetDisplayName()),
	}
	if updated.GetId() != nil {
		result.ID = *updated.GetId()
	}
	return &result, nil
}

// DeleteMailFolder deletes a mail folder.
func (c *Client) DeleteMailFolder(ctx context.Context, folderID string) error {
	if err := validateID(folderID, "folder ID"); err != nil {
		return err
	}
	err := c.inner.Me().MailFolders().ByMailFolderId(folderID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting mail folder: %w", err)
	}
	return nil
}

func (c *Client) SearchMessages(ctx context.Context, query string, top int32) ([]MailMessage, error) {
	return c.ListMessages(ctx, &ListMessagesOptions{
		Top:    top,
		Search: fmt.Sprintf(`"%s"`, strings.ReplaceAll(query, `"`, ``)), //nolint:gocritic // %q adds Go escapes that break Graph API $search syntax
	})
}

// Attachment represents a mail attachment
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int32  `json:"size"`
	Content     []byte `json:"-"`
}

// AttachmentInput represents an attachment to be sent with a message
type AttachmentInput struct {
	Name        string
	ContentType string
	Content     []byte
}

func (c *Client) DownloadAttachment(ctx context.Context, messageID, attachmentID string) (*Attachment, error) {
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	if err := validateID(attachmentID, "attachment ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Me().Messages().ByMessageId(messageID).Attachments().ByAttachmentId(attachmentID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading attachment: %w", err)
	}

	att := &Attachment{
		Name:        derefStr(resp.GetName()),
		ContentType: derefStr(resp.GetContentType()),
	}
	if resp.GetId() != nil {
		att.ID = *resp.GetId()
	}
	if resp.GetSize() != nil {
		att.Size = *resp.GetSize()
	}

	// Type-assert to FileAttachmentable to get content bytes
	if fileAtt, ok := resp.(models.FileAttachmentable); ok {
		att.Content = fileAtt.GetContentBytes()
	} else {
		return nil, fmt.Errorf("attachment %q is not a file attachment", att.Name)
	}

	return att, nil
}

func (c *Client) GetAttachments(ctx context.Context, messageID string) ([]Attachment, error) {
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	resp, err := c.inner.Me().Messages().ByMessageId(messageID).Attachments().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting attachments: %w", err)
	}

	var attachments []Attachment
	for _, a := range resp.GetValue() {
		att := Attachment{
			Name:        derefStr(a.GetName()),
			ContentType: derefStr(a.GetContentType()),
		}
		if a.GetId() != nil {
			att.ID = *a.GetId()
		}
		if a.GetSize() != nil {
			att.Size = *a.GetSize()
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

// FlagMessage sets the follow-up flag status on a message
func (c *Client) FlagMessage(ctx context.Context, messageID, flagStatus string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}

	flag := models.NewFollowupFlag()
	var status models.FollowupFlagStatus
	switch flagStatus {
	case "flagged":
		status = models.FLAGGED_FOLLOWUPFLAGSTATUS
	case "complete":
		status = models.COMPLETE_FOLLOWUPFLAGSTATUS
	case "notFlagged":
		status = models.NOTFLAGGED_FOLLOWUPFLAGSTATUS
	default:
		return fmt.Errorf("invalid flag status: %q (must be flagged, complete, or notFlagged)", flagStatus)
	}
	flag.SetFlagStatus(&status)

	msg := models.NewMessage()
	msg.SetFlag(flag)

	_, err := c.inner.Me().Messages().ByMessageId(messageID).Patch(ctx, msg, nil)
	if err != nil {
		return fmt.Errorf("flagging message: %w", err)
	}
	return nil
}

// SetImportance sets the importance level on a message
func (c *Client) SetImportance(ctx context.Context, messageID, importance string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}

	var imp models.Importance
	switch importance {
	case importanceLow:
		imp = models.LOW_IMPORTANCE
	case importanceNormal:
		imp = models.NORMAL_IMPORTANCE
	case importanceHigh:
		imp = models.HIGH_IMPORTANCE
	default:
		return fmt.Errorf("invalid importance: %q (must be low, normal, or high)", importance)
	}

	msg := models.NewMessage()
	msg.SetImportance(&imp)

	_, err := c.inner.Me().Messages().ByMessageId(messageID).Patch(ctx, msg, nil)
	if err != nil {
		return fmt.Errorf("setting importance: %w", err)
	}
	return nil
}

// CategorizeMessage sets the categories on a message
func (c *Client) CategorizeMessage(ctx context.Context, messageID string, categories []string) error {
	if err := validateID(messageID, "message ID"); err != nil {
		return err
	}

	msg := models.NewMessage()
	msg.SetCategories(categories)

	_, err := c.inner.Me().Messages().ByMessageId(messageID).Patch(ctx, msg, nil)
	if err != nil {
		return fmt.Errorf("categorizing message: %w", err)
	}
	return nil
}

func convertMessage(msg models.Messageable) MailMessage {
	m := MailMessage{}
	if msg.GetId() != nil {
		m.ID = *msg.GetId()
	}
	if msg.GetSubject() != nil {
		m.Subject = *msg.GetSubject()
	}
	if msg.GetFrom() != nil && msg.GetFrom().GetEmailAddress() != nil {
		addr := msg.GetFrom().GetEmailAddress()
		if addr.GetAddress() != nil {
			m.From = *addr.GetAddress()
		}
	}
	for _, r := range msg.GetToRecipients() {
		if r.GetEmailAddress() != nil && r.GetEmailAddress().GetAddress() != nil {
			m.To = append(m.To, *r.GetEmailAddress().GetAddress())
		}
	}
	if msg.GetReceivedDateTime() != nil {
		m.ReceivedAt = msg.GetReceivedDateTime().Format("2006-01-02T15:04:05Z")
	}
	if msg.GetIsRead() != nil {
		m.IsRead = *msg.GetIsRead()
	}
	if msg.GetHasAttachments() != nil {
		m.HasAttachments = *msg.GetHasAttachments()
	}
	if msg.GetBodyPreview() != nil {
		m.BodyPreview = *msg.GetBodyPreview()
	}
	if cats := msg.GetCategories(); len(cats) > 0 {
		m.Categories = cats
	}
	return m
}

func makeRecipients(emails []string) ([]models.Recipientable, error) {
	var recipients []models.Recipientable
	for _, email := range emails {
		if err := ValidateEmail(email); err != nil {
			return nil, err
		}
		r := models.NewRecipient()
		addr := models.NewEmailAddress()
		e := email
		addr.SetAddress(&e)
		r.SetEmailAddress(addr)
		recipients = append(recipients, r)
	}
	return recipients, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
