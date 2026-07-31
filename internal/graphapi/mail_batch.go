package graphapi

import (
	"context"
	"fmt"
	"sort"

	graphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// maxBatchMessages is Graph's per-$batch request limit.
const maxBatchMessages = 20

// GetMessagesBatch fetches several messages in a single Graph $batch round-trip
// (up to maxBatchMessages), instead of one request each. It is best-effort: an
// id that fails (not found / no access) is omitted from the result rather than
// failing the whole call, so a missing id in the output means that fetch failed.
func (c *Client) GetMessagesBatch(ctx context.Context, target string, ids []string, preference MessageBodyPreference) ([]MailMessage, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no message IDs provided")
	}
	if len(ids) > maxBatchMessages {
		return nil, fmt.Errorf("too many message IDs: %d (max %d per batch)", len(ids), maxBatchMessages)
	}
	for _, id := range ids {
		if err := validateID(id, "message ID"); err != nil {
			return nil, err
		}
	}
	preferenceValue, err := preference.headerValue()
	if err != nil {
		return nil, err
	}

	adapter := c.inner.GetAdapter()
	batch := graphcore.NewBatchRequest(adapter)
	cfg := &users.ItemMessagesMessageItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemMessagesMessageItemRequestBuilderGetQueryParameters{
			Select: messageDetailSelect,
		},
	}

	// Preserve request order and map each step id back to its result.
	stepIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		reqInfo, err := c.targetUser(target).Messages().ByMessageId(id).ToGetRequestInformation(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("building batch request: %w", err)
		}
		if preference != MessageBodyDefault {
			reqInfo.Headers.Add(preferHeader, preferenceValue)
		}
		if c.immutableIDs {
			reqInfo.Headers.Add(preferHeader, immutableIDPreference)
		}
		item, err := batch.AddBatchRequestStep(*reqInfo)
		if err != nil {
			return nil, fmt.Errorf("adding batch step: %w", err)
		}
		stepIDs = append(stepIDs, *item.GetId())
	}

	resp, err := batch.Send(ctx, adapter)
	if err != nil {
		return nil, fmt.Errorf("sending batch request: %w", err)
	}

	out := make([]MailMessage, 0, len(stepIDs))
	for _, stepID := range stepIDs {
		if preference != MessageBodyDefault {
			item := resp.GetResponseById(stepID)
			if item == nil || item.GetStatus() == nil || *item.GetStatus() >= 400 {
				return nil, fmt.Errorf("batch message %q did not return a successful provider body response", stepID)
			}
			if err := verifyPreferenceApplied(batchResponseHeader(item.GetHeaders(), preferenceAppliedHeader), preference); err != nil {
				return nil, fmt.Errorf("batch message %q: %w", stepID, err)
			}
		}
		msg, err := graphcore.GetBatchResponseById[models.Messageable](resp, stepID, models.CreateMessageFromDiscriminatorValue)
		if err != nil {
			if preference != MessageBodyDefault {
				return nil, fmt.Errorf("batch message %q: %w", stepID, err)
			}
			continue // best-effort: skip ids that failed (not found / no access)
		}
		m := convertMessage(msg)
		fillBody(&m, msg)
		if err := verifyMessageBody(&m, preference); err != nil {
			return nil, fmt.Errorf("batch message %q: %w", stepID, err)
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no messages returned (all %d ids failed or were not found)", len(ids))
	}
	return out, nil
}

// ListThread returns every message in a conversation, oldest first. The
// conversation id comes from a message's conversationId field (now included in
// list/get output). Messages are matched across all folders (inbox, sent, …).
func (c *Client) ListThread(ctx context.Context, target, conversationID string, top int32, preference MessageBodyPreference) ([]MailMessage, error) {
	if err := validateID(conversationID, "conversation ID"); err != nil {
		return nil, err
	}
	if _, err := preference.headerValue(); err != nil {
		return nil, err
	}
	// validateID's character set excludes quotes, so the id can't break out of
	// the OData string literal — the filter is injection-safe. Graph rejects
	// $orderby alongside a conversationId filter, so order client-side.
	filter := fmt.Sprintf("conversationId eq '%s'", conversationID)
	if preference == MessageBodyDefault {
		// Preserve the historical single-list-request, metadata-only contract.
		// Only an explicit typed body preference opts into exact batch hydration.
		messages, err := c.ListMessages(ctx, target, &ListMessagesOptions{Filter: filter, Top: top})
		if err != nil {
			return nil, err
		}
		return completeThreadMessages(
			conversationID,
			messages,
		)
	}

	metadata, err := c.ListMessages(ctx, target, &ListMessagesOptions{
		Filter: filter,
		Top:    top,
		Select: []string{"id"},
	})
	if err != nil {
		return nil, err
	}
	return c.hydrateThread(
		ctx,
		target,
		conversationID,
		metadata,
		preference,
	)
}

const completeThreadPageSize int32 = 1000

// ListCompleteThread consumes every Graph continuation page before returning
// one exact conversation, oldest first.
func (c *Client) ListCompleteThread(
	ctx context.Context,
	target string,
	conversationID string,
	preference MessageBodyPreference,
) ([]MailMessage, error) {
	if err := validateID(conversationID, "conversation ID"); err != nil {
		return nil, err
	}
	if _, err := preference.headerValue(); err != nil {
		return nil, err
	}
	filter := fmt.Sprintf("conversationId eq '%s'", conversationID)
	selectFields := []string{
		"id",
		"subject",
		"from",
		"toRecipients",
		"ccRecipients",
		"bccRecipients",
		"replyTo",
		"receivedDateTime",
		"isRead",
		"hasAttachments",
		"bodyPreview",
		"categories",
		"conversationId",
	}
	if preference != MessageBodyDefault {
		selectFields = []string{"id"}
	}
	pageTop := completeThreadPageSize
	query := &users.ItemMessagesRequestBuilderGetQueryParameters{
		Top:    &pageTop,
		Filter: &filter,
		Select: selectFields,
	}
	raw, err := c.collectUserMessagePages(ctx, target, completeThreadPageSize, true, query)
	if err != nil {
		return nil, fmt.Errorf("listing complete thread: %w", err)
	}
	metadata := make([]MailMessage, 0, len(raw))
	for _, message := range raw {
		metadata = append(metadata, convertMessage(message))
	}
	if preference == MessageBodyDefault {
		return completeThreadMessages(conversationID, metadata)
	}
	return c.hydrateThread(
		ctx,
		target,
		conversationID,
		metadata,
		preference,
	)
}

func (c *Client) hydrateThread(
	ctx context.Context,
	target string,
	conversationID string,
	metadata []MailMessage,
	preference MessageBodyPreference,
) ([]MailMessage, error) {
	discoveredIDs := make([]string, 0, len(metadata))
	for i := range metadata {
		discoveredIDs = append(discoveredIDs, metadata[i].ID)
	}

	messages := make([]MailMessage, 0, len(discoveredIDs))
	for start := 0; start < len(discoveredIDs); start += maxBatchMessages {
		end := min(start+maxBatchMessages, len(discoveredIDs))
		chunkIDs := discoveredIDs[start:end]
		chunk, err := c.GetMessagesBatch(ctx, target, chunkIDs, preference)
		if err != nil {
			return nil, err
		}
		if err := verifyExactMessageIdentity(chunkIDs, chunk); err != nil {
			return nil, fmt.Errorf("thread batch identity: %w", err)
		}
		messages = append(messages, chunk...)
	}
	if err := verifyExactMessageIdentity(discoveredIDs, messages); err != nil {
		return nil, fmt.Errorf("thread identity: %w", err)
	}
	return completeThreadMessages(conversationID, messages)
}

func completeThreadMessages(
	conversationID string,
	messages []MailMessage,
) ([]MailMessage, error) {
	for i := range messages {
		if messages[i].ConversationID != conversationID {
			return nil, fmt.Errorf(
				"thread message %q conversation = %q, want %q",
				messages[i].ID,
				messages[i].ConversationID,
				conversationID,
			)
		}
	}
	sortThreadMessages(messages)
	return messages, nil
}

func verifyExactMessageIdentity(expected []string, messages []MailMessage) error {
	if len(messages) != len(expected) {
		return fmt.Errorf("returned %d messages, want %d", len(messages), len(expected))
	}
	remaining := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		if _, duplicate := remaining[id]; duplicate {
			return fmt.Errorf("expected message ID %q is duplicated", id)
		}
		remaining[id] = struct{}{}
	}
	for i := range messages {
		if _, found := remaining[messages[i].ID]; !found {
			return fmt.Errorf("unexpected or duplicate message ID %q", messages[i].ID)
		}
		delete(remaining, messages[i].ID)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("missing %d expected message IDs", len(remaining))
	}
	return nil
}

func sortThreadMessages(messages []MailMessage) {
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].ReceivedAt < messages[j].ReceivedAt // RFC3339 Z strings sort chronologically
	})
}
