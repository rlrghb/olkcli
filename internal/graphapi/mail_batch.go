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
func (c *Client) GetMessagesBatch(ctx context.Context, target string, ids []string) ([]MailMessage, error) {
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
		msg, err := graphcore.GetBatchResponseById[models.Messageable](resp, stepID, models.CreateMessageFromDiscriminatorValue)
		if err != nil {
			continue // best-effort: skip ids that failed (not found / no access)
		}
		m := convertMessage(msg)
		fillBody(&m, msg)
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
func (c *Client) ListThread(ctx context.Context, target, conversationID string, top int32) ([]MailMessage, error) {
	if err := validateID(conversationID, "conversation ID"); err != nil {
		return nil, err
	}
	// validateID's character set excludes quotes, so the id can't break out of
	// the OData string literal — the filter is injection-safe. Graph rejects
	// $orderby alongside a conversationId filter, so order client-side.
	filter := fmt.Sprintf("conversationId eq '%s'", conversationID)
	messages, err := c.ListMessages(ctx, target, &ListMessagesOptions{Filter: filter, Top: top})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].ReceivedAt < messages[j].ReceivedAt // RFC3339 Z strings sort chronologically
	})
	return messages, nil
}
