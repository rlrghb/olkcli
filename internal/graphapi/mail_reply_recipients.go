package graphapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// addReplyRecipients sets on patch the Cc and Bcc lists that result from
// adding the caller's addresses to those Graph generated for the reply. A
// PATCH replaces a recipient list outright, so each list is written in full:
// the generated recipients first, then each added address that is not already
// on any of the reply's lists. A list with nothing to add is left out of the
// PATCH.
func addReplyRecipients(patch, generated models.Messageable, opts *CreateReplyDraftOptions) error {
	present := make(map[string]struct{})
	for _, list := range [][]models.Recipientable{
		generated.GetToRecipients(), generated.GetCcRecipients(), generated.GetBccRecipients(),
	} {
		for _, recipient := range list {
			present[strings.ToLower(recipientAddress(recipient))] = struct{}{}
		}
	}
	if len(opts.Cc) > 0 {
		cc, err := withAddedRecipients(generated.GetCcRecipients(), opts.Cc, "Cc", present)
		if err != nil {
			return err
		}
		patch.SetCcRecipients(cc)
	}
	if len(opts.Bcc) > 0 {
		bcc, err := withAddedRecipients(generated.GetBccRecipients(), opts.Bcc, "Bcc", present)
		if err != nil {
			return err
		}
		patch.SetBccRecipients(bcc)
	}
	return nil
}

// withAddedRecipients appends to generated each added address not yet in
// present, recording it there. It refuses to proceed when Graph omitted the
// generated list, because writing only the added addresses would silently
// drop the recipients a reply-all is meant to reach.
func withAddedRecipients(
	generated []models.Recipientable,
	added []string,
	label string,
	present map[string]struct{},
) ([]models.Recipientable, error) {
	if generated == nil {
		return nil, fmt.Errorf("adding %s recipients: Graph did not report the reply's generated %s list, so it cannot be extended safely", label, label)
	}
	extra, err := makeRecipients(added)
	if err != nil {
		return nil, fmt.Errorf("invalid %s recipient: %w", strings.ToLower(label), err)
	}
	merged := append(make([]models.Recipientable, 0, len(generated)+len(extra)), generated...)
	for _, recipient := range extra {
		key := strings.ToLower(recipientAddress(recipient))
		if _, exists := present[key]; exists {
			continue
		}
		present[key] = struct{}{}
		merged = append(merged, recipient)
	}
	return merged, nil
}

func recipientAddress(recipient models.Recipientable) string {
	if recipient == nil || recipient.GetEmailAddress() == nil {
		return ""
	}
	return derefStr(recipient.GetEmailAddress().GetAddress())
}

// finishReplyDraftRecipients adds recipients after draft creation
// has already supplied the body, either as a plain comment or explicit HTML.
func (c *Client) finishReplyDraftRecipients(
	ctx context.Context,
	target, draftID string,
	created models.Messageable,
	opts *CreateReplyDraftOptions,
) (*DraftMessage, error) {
	draft := convertDraft(created)
	if len(opts.Cc) == 0 && len(opts.Bcc) == 0 {
		return &draft, nil
	}
	patch := models.NewMessage()
	if err := addReplyRecipients(patch, created, opts); err != nil {
		return nil, err
	}
	updated, err := c.patchReplyDraft(ctx, target, draftID, patch, replyDraftKind)
	if err != nil {
		return nil, err
	}
	draft.Cc = recipientAddresses(firstNonNil(updated.GetCcRecipients(), patch.GetCcRecipients(), created.GetCcRecipients()))
	draft.Bcc = recipientAddresses(firstNonNil(updated.GetBccRecipients(), patch.GetBccRecipients(), created.GetBccRecipients()))
	return &draft, nil
}

func firstNonNil(lists ...[]models.Recipientable) []models.Recipientable {
	for _, list := range lists {
		if list != nil {
			return list
		}
	}
	return nil
}
