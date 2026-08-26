package graphapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// MaxInlineAttachmentBytes is Graph's exclusive upper bound for a simple
// attachment upload. Larger files require an upload session, which this
// focused inline-image path deliberately does not implement.
const (
	MaxInlineAttachmentBytes = 3 << 20
	messageBodyName          = "body"
)

var (
	inlineContentIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]*$`)
	htmlDocumentTagPattern = regexp.MustCompile(`(?i)<\s*(?:html|body)(?:\s|>)`)
)

// InlineAttachmentInput is an image embedded in an HTML draft and
// referenced from the draft body with a matching cid: URL.
type InlineAttachmentInput struct {
	ContentID   string
	Name        string
	ContentType string
	Content     []byte
}

// CreateReplyDraftOptions carries the content and reply mode for a threaded
// reply draft. InlineAttachments are supported only for HTML drafts.
type CreateReplyDraftOptions struct {
	Body              string
	ReplyAll          bool
	IsHTML            bool
	InlineAttachments []InlineAttachmentInput
}

// ValidateInlineContentID rejects values that cannot be used safely and
// predictably in both a Content-ID header and a cid: URL.
func ValidateInlineContentID(contentID string) error {
	if contentID == "" {
		return fmt.Errorf("inline content ID cannot be empty")
	}
	if !inlineContentIDPattern.MatchString(contentID) {
		return fmt.Errorf("invalid inline content ID %q: use letters, numbers, dot, underscore, at, plus, or hyphen", contentID)
	}
	return nil
}

// HTMLReferencesInlineContentID reports whether HTML contains a cid: URL for
// exactly contentID. The boundary prevents a short ID such as "logo" from
// being accepted merely because the body references "logo-large".
func HTMLReferencesInlineContentID(body, contentID string) bool {
	pattern := regexp.MustCompile(`(?i:cid:)` + regexp.QuoteMeta(contentID) + `(?:["'\s)>]|$)`)
	return pattern.MatchString(body)
}

// CreateReplyDraft creates a real Outlook reply draft in the target mailbox,
// or in the signed-in user's own mailbox when target is empty. Plain drafts use
// Graph's comment form. HTML drafts first let Graph generate Outlook's quoted
// history, then insert the supplied fragment into that generated HTML body.
func (c *Client) CreateReplyDraft(ctx context.Context, target, messageID string, opts *CreateReplyDraftOptions) (*DraftMessage, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	if err := validateID(messageID, "message ID"); err != nil {
		return nil, err
	}
	if err := validateCreateReplyDraftOptions(opts); err != nil {
		return nil, err
	}

	action := "creating reply draft"
	if opts.ReplyAll {
		action = "creating reply-all draft"
	}

	result, err := c.createReplyDraft(ctx, target, messageID, opts, action)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%s: Graph returned no draft", action)
	}

	draftID := derefStr(result.GetId())
	if draftID == "" {
		return nil, fmt.Errorf("%s: Graph returned a draft without an ID", action)
	}
	if !opts.IsHTML {
		draft := convertDraft(result)
		return &draft, nil
	}

	draft, err := c.finishHTMLReplyDraft(ctx, target, draftID, result, opts)
	if err != nil {
		return nil, c.cleanupFailedDraft(ctx, target, draftID, "reply draft", err)
	}
	return draft, nil
}

func validateCreateReplyDraftOptions(opts *CreateReplyDraftOptions) error {
	if opts == nil {
		return fmt.Errorf("reply draft options are required")
	}
	if len(opts.InlineAttachments) > 0 && !opts.IsHTML {
		return fmt.Errorf("inline attachments require an HTML reply draft")
	}
	if opts.IsHTML && htmlDocumentTagPattern.MatchString(opts.Body) {
		return fmt.Errorf("HTML reply body must be a fragment, not a complete html or body document")
	}
	return validateInlineAttachments(opts.Body, opts.IsHTML, opts.InlineAttachments)
}

func validateInlineAttachments(body string, isHTML bool, attachments []InlineAttachmentInput) error {
	if len(attachments) > 0 && !isHTML {
		return fmt.Errorf("inline attachments require HTML content")
	}
	seen := make(map[string]struct{}, len(attachments))
	for _, attachment := range attachments {
		if err := ValidateInlineContentID(attachment.ContentID); err != nil {
			return err
		}
		key := strings.ToLower(attachment.ContentID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate inline content ID %q", attachment.ContentID)
		}
		seen[key] = struct{}{}
		if strings.TrimSpace(attachment.Name) == "" {
			return fmt.Errorf("inline attachment %q has no filename", attachment.ContentID)
		}
		if !strings.HasPrefix(strings.ToLower(attachment.ContentType), "image/") {
			return fmt.Errorf("inline attachment %q has non-image content type %q", attachment.ContentID, attachment.ContentType)
		}
		if len(attachment.Content) == 0 {
			return fmt.Errorf("inline attachment %q is empty", attachment.ContentID)
		}
		if len(attachment.Content) >= MaxInlineAttachmentBytes {
			return fmt.Errorf("inline attachment %q is %d bytes; Graph simple attachments must be under 3 MB", attachment.ContentID, len(attachment.Content))
		}
		if !HTMLReferencesInlineContentID(body, attachment.ContentID) {
			return fmt.Errorf("HTML body does not reference inline content ID %q as cid:%s", attachment.ContentID, attachment.ContentID)
		}
	}
	return nil
}

func (c *Client) createReplyDraft(
	ctx context.Context,
	target, messageID string,
	opts *CreateReplyDraftOptions,
	action string,
) (models.Messageable, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	message := c.targetUser(target).Messages().ByMessageId(messageID)
	var (
		result models.Messageable
		err    error
	)
	switch {
	case opts.ReplyAll && opts.IsHTML:
		result, err = message.CreateReplyAll().Post(ctx, nil, nil)
	case opts.ReplyAll:
		body := users.NewItemMessagesItemCreateReplyAllPostRequestBody()
		body.SetComment(&opts.Body)
		result, err = message.CreateReplyAll().Post(ctx, body, nil)
	case opts.IsHTML:
		result, err = message.CreateReply().Post(ctx, nil, nil)
	default:
		body := users.NewItemMessagesItemCreateReplyPostRequestBody()
		body.SetComment(&opts.Body)
		result, err = message.CreateReply().Post(ctx, body, nil)
	}
	if err != nil {
		return nil, c.replyDraftError(action, target, err)
	}
	return result, nil
}

func (c *Client) finishHTMLReplyDraft(
	ctx context.Context,
	target, draftID string,
	created models.Messageable,
	opts *CreateReplyDraftOptions,
) (*DraftMessage, error) {
	generated := created
	generatedHTML, insertionIndex, ok := replyDraftHTMLInsertionPoint(generated)
	if !ok {
		var err error
		generated, err = c.getReplyDraftHTML(ctx, target, draftID)
		if err != nil {
			return nil, err
		}
		generatedHTML, insertionIndex, ok = replyDraftHTMLInsertionPoint(generated)
		if !ok {
			return nil, fmt.Errorf("reading generated reply draft %s: Graph did not return a usable HTML body with a body element", draftID)
		}
	}

	combinedHTML := generatedHTML[:insertionIndex] + opts.Body + generatedHTML[insertionIndex:]
	updated, err := c.patchReplyDraftHTML(ctx, target, draftID, combinedHTML)
	if err != nil {
		return nil, err
	}

	for _, attachment := range opts.InlineAttachments {
		if err := c.addInlineReplyDraftAttachment(ctx, target, draftID, attachment); err != nil {
			return nil, err
		}
	}

	draft := convertDraft(updated)
	createdDraft := convertDraft(generated)
	if draft.ID == "" {
		draft.ID = draftID
	}
	if draft.Subject == "" {
		draft.Subject = createdDraft.Subject
	}
	if len(draft.To) == 0 {
		draft.To = createdDraft.To
	}
	if draft.Created == "" {
		draft.Created = createdDraft.Created
	}
	draft.Body = combinedHTML
	return &draft, nil
}

func (c *Client) getReplyDraftHTML(ctx context.Context, target, draftID string) (models.Messageable, error) {
	headers, options, contract, err := newMessageBodyResponseContract(MessageBodyHTML)
	if err != nil {
		return nil, err
	}
	result, err := c.targetUser(target).Messages().ByMessageId(draftID).Get(ctx, &users.ItemMessagesMessageItemRequestBuilderGetRequestConfiguration{
		Headers:         c.messageIDHeaders(headers),
		Options:         options,
		QueryParameters: &users.ItemMessagesMessageItemRequestBuilderGetQueryParameters{Select: messageDetailSelect},
	})
	if err != nil {
		return nil, c.replyDraftError("reading generated reply draft", target, err)
	}
	if err := contract.verify(); err != nil {
		return nil, fmt.Errorf("reading generated reply draft: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("reading generated reply draft: Graph returned no draft")
	}
	return result, nil
}

func (c *Client) patchReplyDraftHTML(ctx context.Context, target, draftID, content string) (models.Messageable, error) {
	if err := c.ensureWritable(); err != nil {
		return nil, err
	}
	message := htmlMessageBody(content)
	result, err := c.targetUser(target).Messages().ByMessageId(draftID).Patch(ctx, message, nil)
	if err != nil {
		return nil, c.replyDraftError("formatting reply draft", target, err)
	}
	if result == nil {
		return nil, fmt.Errorf("formatting reply draft: Graph returned no draft")
	}
	return result, nil
}

func (c *Client) addInlineReplyDraftAttachment(ctx context.Context, target, draftID string, attachment InlineAttachmentInput) error {
	if err := c.ensureWritable(); err != nil {
		return err
	}
	fileAttachment := newInlineFileAttachment(attachment)
	_, err := c.targetUser(target).Messages().ByMessageId(draftID).Attachments().Post(ctx, fileAttachment, nil)
	if err != nil {
		return c.replyDraftError(fmt.Sprintf("adding inline attachment %q to reply draft", attachment.ContentID), target, err)
	}
	return nil
}

func newInlineFileAttachment(attachment InlineAttachmentInput) models.FileAttachmentable {
	fileAttachment := models.NewFileAttachment()
	name := attachment.Name
	contentType := attachment.ContentType
	contentID := attachment.ContentID
	isInline := true
	fileAttachment.SetName(&name)
	fileAttachment.SetContentType(&contentType)
	fileAttachment.SetContentBytes(attachment.Content)
	fileAttachment.SetIsInline(&isInline)
	fileAttachment.SetContentId(&contentID)
	return fileAttachment
}

func (c *Client) replyDraftError(action, target string, err error) error {
	if target != "" {
		return sharedMailboxReplyDraftError(action, target, err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (c *Client) cleanupFailedDraft(ctx context.Context, target, draftID, kind string, cause error) error {
	if err := c.ensureWritable(); err != nil {
		return fmt.Errorf("%w\n\nCleanup also failed; %s %s still exists: %w", cause, kind, draftID, err)
	}
	cleanupErr := c.targetUser(target).Messages().ByMessageId(draftID).Delete(ctx, nil)
	if cleanupErr == nil {
		return fmt.Errorf("%w\n\nThe incomplete %s %s was deleted", cause, kind, draftID)
	}
	return fmt.Errorf("%w\n\nCleanup also failed; %s %s still exists: %s", cause, kind, draftID, graphErrorMessage(cleanupErr))
}

func replyDraftHTMLInsertionPoint(message models.Messageable) (content string, insertionIndex int, ok bool) {
	if message == nil || message.GetBody() == nil || message.GetBody().GetContent() == nil {
		return "", 0, false
	}
	if contentType := message.GetBody().GetContentType(); contentType != nil && *contentType != models.HTML_BODYTYPE {
		return "", 0, false
	}
	content = *message.GetBody().GetContent()
	insertionIndex, ok = htmlBodyStartTagEnd(content)
	return content, insertionIndex, ok
}

// htmlBodyStartTagEnd finds the byte immediately after the real opening body
// tag while preserving every byte Graph generated. It skips comments and the
// contents of head-level script/style blocks, and scans quoted attributes so a
// greater-than sign inside an attribute cannot become the insertion point.
func htmlBodyStartTagEnd(document string) (int, bool) {
	for offset := 0; offset < len(document); {
		relative := strings.IndexByte(document[offset:], '<')
		if relative < 0 {
			return 0, false
		}
		start := offset + relative
		if strings.HasPrefix(document[start:], "<!--") {
			end := strings.Index(document[start+4:], "-->")
			if end < 0 {
				return 0, false
			}
			offset = start + 4 + end + 3
			continue
		}

		nameStart := start + 1
		if nameStart >= len(document) || document[nameStart] == '/' || document[nameStart] == '!' || document[nameStart] == '?' {
			offset = nameStart
			continue
		}
		nameEnd := nameStart
		for nameEnd < len(document) && isHTMLTagNameByte(document[nameEnd]) {
			nameEnd++
		}
		if nameEnd == nameStart {
			offset = nameStart
			continue
		}
		tagEnd, ok := htmlTagEnd(document, nameEnd)
		if !ok {
			return 0, false
		}
		name := strings.ToLower(document[nameStart:nameEnd])
		if name == messageBodyName {
			return tagEnd + 1, true
		}
		if name == "script" || name == "style" || name == "template" {
			closing := "</" + name
			end := strings.Index(strings.ToLower(document[tagEnd+1:]), closing)
			if end < 0 {
				return 0, false
			}
			offset = tagEnd + 1 + end + len(closing)
			continue
		}
		offset = tagEnd + 1
	}
	return 0, false
}

func htmlTagEnd(document string, offset int) (int, bool) {
	var quote byte
	for i := offset; i < len(document); i++ {
		char := document[i]
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == '>' {
			return i, true
		}
	}
	return 0, false
}

func isHTMLTagNameByte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' || value == ':'
}
