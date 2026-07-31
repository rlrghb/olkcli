package graphapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

var (
	errNilMailFolderResponse = errors.New("graph returned no mail folder response")
	errNilMessageResponse    = errors.New("graph returned no message response")
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

// maxAttachmentBytes is the hard limit for attachment downloads (50 MB).
// Applied in the API layer so oversized content is rejected before reaching callers.
const maxAttachmentBytes = 50 << 20

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
	ID             string            `json:"id"`
	ParentFolderID string            `json:"parentFolderId,omitempty"`
	ChangeKey      string            `json:"changeKey,omitempty"`
	Subject        string            `json:"subject" untrusted:"true"`
	From           string            `json:"from" untrusted:"true"`
	To             []string          `json:"to" untrusted:"true"`
	Cc             []string          `json:"cc" untrusted:"true"`
	Bcc            []string          `json:"bcc" untrusted:"true"`
	ReplyTo        []string          `json:"replyTo" untrusted:"true"`
	ReceivedAt     string            `json:"receivedDateTime"`
	IsRead         bool              `json:"isRead"`
	HasAttachments bool              `json:"hasAttachments"`
	BodyPreview    string            `json:"bodyPreview,omitempty" untrusted:"true" concise:"omit"`
	Body           string            `json:"body,omitempty" untrusted:"true" concise:"omit"`
	BodyType       string            `json:"bodyType,omitempty"`
	Categories     []string          `json:"categories,omitempty"`
	ConversationID string            `json:"conversationId,omitempty"`
	Flag           *MailFollowupFlag `json:"flag,omitempty"`
}

// MailFollowupFlag is the stable output shape for a provider follow-up flag.
type MailFollowupFlag struct {
	Status string `json:"status"`
}

// messageDetailSelect is the $select field set for a full single message (used by
// GetMessage and the batch fetch) — includes the body and conversation id.
var messageDetailSelect = []string{
	"id", "subject", "from", "toRecipients", "ccRecipients", "bccRecipients",
	"receivedDateTime", "isRead", "hasAttachments", "body", "bodyPreview", "conversationId",
	"parentFolderId", "changeKey", "flag",
}

// MailFolder is a simplified folder representation
type MailFolder struct {
	ID             string `json:"id"`
	WellKnownName  string `json:"wellKnownName,omitempty"`
	DisplayName    string `json:"displayName" untrusted:"true"`
	TotalCount     int32  `json:"totalItemCount"`
	UnreadCount    int32  `json:"unreadItemCount"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
}

// protectedWellKnownMailFolders is the canonical folder set used by guarded
// mailbox moves. Each value is resolved through Graph's well-known-name route;
// display names are never interpreted as identity.
var protectedWellKnownMailFolders = map[string]bool{
	"archive":      true,
	"deleteditems": true,
	"inbox":        true,
	"junkemail":    true,
}

// MoveMessageReceipt is the stable provider-success result returned after
// Graph has created the destination message.
type MoveMessageReceipt struct {
	SourceID string `json:"sourceId"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	Code     string `json:"code"`
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

// ListMessages returns messages from the target mailbox, or the signed-in user's
// mailbox when target is empty. Targeting another mailbox requires the calling
// token to carry the Mail.Read.Shared scope claim and the calling user to have
// Full Access delegation on the target mailbox in Exchange.
func (c *Client) ListMessages(ctx context.Context, target string, opts *ListMessagesOptions) ([]MailMessage, error) {
	if opts == nil {
		opts = &ListMessagesOptions{}
	}
	top := clampTop(opts.Top)

	hasInferenceClassification := strings.Contains(opts.Filter, "inferenceClassification")
	if opts.OrderBy != "" && hasInferenceClassification {
		return nil, fmt.Errorf("cannot combine orderBy with inferenceClassification filter")
	}
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "receivedDateTime desc"
	}
	if !allowedOrderBy[orderBy] {
		return nil, fmt.Errorf("invalid orderBy value: %q", orderBy)
	}

	queryParams := &users.ItemMessagesRequestBuilderGetQueryParameters{
		Top: &top,
	}
	// Microsoft Graph does not support $orderBy combined with $search, an
	// inferenceClassification filter, or a conversationId filter ("restriction or
	// sort order is too complex"). Explicit classification ordering is rejected
	// above; search and conversation callers retain their existing semantics.
	skipOrderBy := opts.Search != "" || hasInferenceClassification || strings.Contains(opts.Filter, "conversationId")
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
		queryParams.Select = append([]string(nil), opts.Select...)
	} else {
		queryParams.Select = []string{
			"id", "subject", "from", "toRecipients", "ccRecipients",
			"bccRecipients", "replyTo", "receivedDateTime", "isRead",
			"hasAttachments", "bodyPreview", "categories", "conversationId",
		}
	}

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
		messages, err := collectMessagePages(ctx, top,
			func(ctx context.Context, pageTop int32) (messagePage, error) {
				folderQueryParams.Top = &pageTop
				resp, err := c.targetUser(target).MailFolders().ByMailFolderId(opts.FolderID).Messages().Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
					Headers:         c.messageIDHeaders(nil),
					QueryParameters: folderQueryParams,
				})
				if err != nil {
					return messagePage{}, err
				}
				if resp == nil {
					return messagePage{}, errNilMessageResponse
				}
				return messagePage{Values: resp.GetValue(), NextLink: derefStr(resp.GetOdataNextLink())}, nil
			},
			func(ctx context.Context, nextLink string, _ int32) (messagePage, error) {
				if err := validateGraphContinuation(nextLink, graphContinuationScope{
					host:           defaultGraphAPIHost,
					collectionPath: graphUserCollectionPath(target, "mailFolders/"+url.PathEscape(opts.FolderID)+"/messages"),
				}); err != nil {
					return messagePage{}, err
				}
				resp, err := users.NewItemMailFoldersItemMessagesRequestBuilder(nextLink, c.inner.GetAdapter()).Get(ctx, &users.ItemMailFoldersItemMessagesRequestBuilderGetRequestConfiguration{
					Headers: c.messageIDHeaders(nil),
				})
				if err != nil {
					return messagePage{}, err
				}
				if resp == nil {
					return messagePage{}, errNilMessageResponse
				}
				return messagePage{Values: resp.GetValue(), NextLink: derefStr(resp.GetOdataNextLink())}, nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("listing messages: %w", err)
		}
		result := make([]MailMessage, 0, len(messages))
		for _, msg := range messages {
			result = append(result, convertMessage(msg))
		}
		return result, nil
	}

	messages, err := c.collectUserMessagePages(ctx, target, top, false, queryParams)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	result := make([]MailMessage, 0, len(messages))
	for _, msg := range messages {
		result = append(result, convertMessage(msg))
	}
	return result, nil
}

// GetMessage returns a single message from the target mailbox, or the signed-in
// user's mailbox when target is empty. See ListMessages for scope requirements.
func (c *Client) GetMessage(ctx context.Context, target, messageID string, preference MessageBodyPreference) (*MailMessage, error) {
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	headers, options, contract, err := newMessageBodyResponseContract(preference)
	if err != nil {
		return nil, err
	}
	msg, err := c.targetUser(target).Messages().ByMessageId(messageID).Get(ctx, &users.ItemMessagesMessageItemRequestBuilderGetRequestConfiguration{
		Headers: c.messageIDHeaders(headers),
		Options: options,
		QueryParameters: &users.ItemMessagesMessageItemRequestBuilderGetQueryParameters{
			Select: messageDetailSelect,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting message: %w", err)
	}
	if err := contract.verify(); err != nil {
		return nil, fmt.Errorf("getting message: %w", err)
	}
	m := convertMessage(msg)
	fillBody(&m, msg)
	if err := verifyMessageBody(&m, preference); err != nil {
		return nil, fmt.Errorf("getting message: %w", err)
	}
	return &m, nil
}

func (c *Client) SendMessage(ctx context.Context, subject, body string, toRecipients, ccRecipients, bccRecipients []string, isHTML bool, attachments []AttachmentInput, importance string, readReceipt bool) error {
	if err := c.ensureMaySend(); err != nil {
		return err
	}
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
	if err := c.ensureMaySend(); err != nil {
		return err
	}
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
	if err := c.ensureMaySend(); err != nil {
		return err
	}
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

func (c *Client) MoveMessage(ctx context.Context, messageID, folderID string) (*MoveMessageReceipt, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	if err := validateID(folderID, "folder ID"); err != nil {
		return nil, err
	}
	body := users.NewItemMessagesItemMovePostRequestBody()
	body.SetDestinationId(&folderID)

	moved, err := c.inner.Me().Messages().ByMessageId(messageID).Move().Post(
		ctx,
		body,
		&users.ItemMessagesItemMoveRequestBuilderPostRequestConfiguration{
			Headers: c.messageIDHeaders(nil),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("move message: %w", err)
	}
	if moved == nil || moved.GetId() == nil || *moved.GetId() == "" {
		return nil, fmt.Errorf("move message: %w", errNilMessageResponse)
	}
	return &MoveMessageReceipt{
		SourceID: messageID,
		ID:       *moved.GetId(),
		Status:   "succeeded",
		Code:     "move_succeeded",
	}, nil
}

func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
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
	if err := c.ensureWritable(); err != nil {
		return err
	}
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

// ListMailFolders returns folders from the target mailbox, or the signed-in
// user's mailbox when target is empty. See ListMessages for scope requirements.
func (c *Client) ListMailFolders(ctx context.Context, target string) ([]MailFolder, error) {
	var top int32 = 100
	resp, err := c.targetUser(target).MailFolders().Get(ctx, &users.ItemMailFoldersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMailFoldersRequestBuilderGetQueryParameters{
			Top: &top,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}

	folders := make([]MailFolder, 0, len(resp.GetValue()))
	for _, f := range resp.GetValue() {
		folders = append(folders, convertMailFolder(f))
	}
	return folders, nil
}

// GetWellKnownMailFolder resolves one guarded move destination by its canonical
// Graph identifier. It intentionally does not infer identity from localized or
// user-editable display names.
func (c *Client) GetWellKnownMailFolder(ctx context.Context, target, name string) (*MailFolder, error) {
	if !protectedWellKnownMailFolders[name] {
		return nil, fmt.Errorf("unsupported well-known mail folder %q", name)
	}
	value, err := c.targetUser(target).MailFolders().ByMailFolderId(name).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting well-known folder %s: %w", name, err)
	}
	if value == nil {
		return nil, fmt.Errorf("getting well-known folder %s: %w", name, errNilMailFolderResponse)
	}
	folder := convertMailFolder(value)
	if folder.ID == "" {
		return nil, fmt.Errorf("getting well-known folder %s: missing folder ID", name)
	}
	folder.WellKnownName = name
	return &folder, nil
}

// CreateMailFolder creates a new mail folder.
func (c *Client) CreateMailFolder(ctx context.Context, displayName string) (*MailFolder, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
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
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
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
	if err := c.ensureWritable(); err != nil {
		return err
	}
	if err := validateID(folderID, "folder ID"); err != nil {
		return err
	}
	err := c.inner.Me().MailFolders().ByMailFolderId(folderID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting mail folder: %w", err)
	}
	return nil
}

// SearchMessages runs a KQL search against the target mailbox, or the signed-in
// user's mailbox when target is empty. See ListMessages for scope requirements.
func (c *Client) SearchMessages(ctx context.Context, target, query string, top int32) ([]MailMessage, error) {
	// The $search parameter value must be wrapped in double quotes per
	// Graph API requirements. KQL property restrictions (from:, subject:, etc.)
	// and boolean operators (AND, OR, NOT) work inside the quoted string.
	// Strip literal double quotes from user input to prevent breaking the wrapper.
	search := `"` + strings.ReplaceAll(query, `"`, "") + `"`
	return c.ListMessages(ctx, target, &ListMessagesOptions{
		Top:    top,
		Search: search,
	})
}

// Attachment represents a mail attachment
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name" untrusted:"true"`
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

	if len(att.Content) > maxAttachmentBytes {
		return nil, fmt.Errorf("attachment %q is %d bytes, exceeds %d byte limit", att.Name, len(att.Content), maxAttachmentBytes)
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

	attachments := make([]Attachment, 0, len(resp.GetValue()))
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
	if err := c.ensureWritable(); err != nil {
		return err
	}
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
	if err := c.ensureWritable(); err != nil {
		return err
	}
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
	if err := c.ensureWritable(); err != nil {
		return err
	}
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
	m := MailMessage{
		To:      []string{},
		Cc:      []string{},
		Bcc:     []string{},
		ReplyTo: []string{},
	}
	if msg.GetId() != nil {
		m.ID = *msg.GetId()
	}
	if msg.GetParentFolderId() != nil {
		m.ParentFolderID = *msg.GetParentFolderId()
	}
	if msg.GetChangeKey() != nil {
		m.ChangeKey = *msg.GetChangeKey()
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
	m.To = recipientAddresses(msg.GetToRecipients())
	m.Cc = recipientAddresses(msg.GetCcRecipients())
	m.Bcc = recipientAddresses(msg.GetBccRecipients())
	m.ReplyTo = recipientAddresses(msg.GetReplyTo())
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
	if msg.GetConversationId() != nil {
		m.ConversationID = *msg.GetConversationId()
	}
	if flag := msg.GetFlag(); flag != nil && flag.GetFlagStatus() != nil {
		m.Flag = &MailFollowupFlag{Status: flag.GetFlagStatus().String()}
	}
	return m
}

func recipientAddresses(recipients []models.Recipientable) []string {
	result := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.GetEmailAddress() != nil &&
			recipient.GetEmailAddress().GetAddress() != nil {
			result = append(
				result,
				*recipient.GetEmailAddress().GetAddress(),
			)
		}
	}
	return result
}

func convertMailFolder(value models.MailFolderable) MailFolder {
	folder := MailFolder{
		DisplayName: derefStr(value.GetDisplayName()),
	}
	if value.GetId() != nil {
		folder.ID = *value.GetId()
	}
	if value.GetTotalItemCount() != nil {
		folder.TotalCount = *value.GetTotalItemCount()
	}
	if value.GetUnreadItemCount() != nil {
		folder.UnreadCount = *value.GetUnreadItemCount()
	}
	if value.GetParentFolderId() != nil {
		folder.ParentFolderID = *value.GetParentFolderId()
	}
	return folder
}

// fillBody copies a message's body content and type into m (convertMessage only
// sets the preview, since list responses don't include the full body).
func fillBody(m *MailMessage, msg models.Messageable) {
	if msg.GetBody() == nil {
		return
	}
	if content := msg.GetBody().GetContent(); content != nil {
		m.Body = *content
	}
	if ct := msg.GetBody().GetContentType(); ct != nil {
		m.BodyType = ct.String()
	}
}

func makeRecipients(emails []string) ([]models.Recipientable, error) {
	recipients := make([]models.Recipientable, 0, len(emails))
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
