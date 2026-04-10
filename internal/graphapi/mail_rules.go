package graphapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// MailRule is a simplified message rule for output
type MailRule struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Sequence    int32  `json:"sequence"`
	IsEnabled   bool   `json:"isEnabled"`
	Conditions  string `json:"conditions"`
	Actions     string `json:"actions"`
}

func (c *Client) ListMailRules(ctx context.Context) ([]MailRule, error) {
	resp, err := c.inner.Me().MailFolders().ByMailFolderId("inbox").MessageRules().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("listing mail rules: %s (note: this feature requires a work/school account)", graphErrorMessage(err))
	}

	var rules []MailRule
	for _, r := range resp.GetValue() {
		rules = append(rules, convertMailRule(r))
	}
	return rules, nil
}

func (c *Client) CreateMailRule(ctx context.Context, name string, from string, subjectContains string, hasAttachment bool, moveFolder string, markRead bool, deleteMsg bool, forwardTo string, importance string) (*MailRule, error) {
	rule := models.NewMessageRule()
	rule.SetDisplayName(&name)
	enabled := true
	rule.SetIsEnabled(&enabled)
	seq := int32(1)
	rule.SetSequence(&seq)

	// Conditions
	conditions := models.NewMessageRulePredicates()
	hasConditions := false

	if from != "" {
		if err := ValidateEmail(from); err != nil {
			return nil, fmt.Errorf("invalid --from: %w", err)
		}
		addr := models.NewRecipient()
		emailAddr := models.NewEmailAddress()
		emailAddr.SetAddress(&from)
		addr.SetEmailAddress(emailAddr)
		conditions.SetFromAddresses([]models.Recipientable{addr})
		hasConditions = true
	}
	if subjectContains != "" {
		conditions.SetSubjectContains([]string{subjectContains})
		hasConditions = true
	}
	if hasAttachment {
		conditions.SetHasAttachments(&hasAttachment)
		hasConditions = true
	}

	if !hasConditions {
		return nil, fmt.Errorf("at least one condition is required")
	}
	rule.SetConditions(conditions)

	// Actions
	actions := models.NewMessageRuleActions()

	if moveFolder != "" {
		if err := validateID(moveFolder, "move folder"); err != nil {
			return nil, err
		}
		actions.SetMoveToFolder(&moveFolder)
	}
	if markRead {
		actions.SetMarkAsRead(&markRead)
	}
	if deleteMsg {
		actions.SetDelete(&deleteMsg)
	}
	if forwardTo != "" {
		if err := ValidateEmail(forwardTo); err != nil {
			return nil, fmt.Errorf("invalid --forward-to: %w", err)
		}
		addr := models.NewRecipient()
		emailAddr := models.NewEmailAddress()
		emailAddr.SetAddress(&forwardTo)
		addr.SetEmailAddress(emailAddr)
		actions.SetForwardTo([]models.Recipientable{addr})
	}
	if importance != "" {
		var imp models.Importance
		switch importance {
		case "low":
			imp = models.LOW_IMPORTANCE
		case "normal":
			imp = models.NORMAL_IMPORTANCE
		case "high":
			imp = models.HIGH_IMPORTANCE
		default:
			return nil, fmt.Errorf("invalid importance: %q", importance)
		}
		actions.SetMarkImportance(&imp)
	}
	rule.SetActions(actions)

	created, err := c.inner.Me().MailFolders().ByMailFolderId("inbox").MessageRules().Post(ctx, rule, nil)
	if err != nil {
		return nil, fmt.Errorf("creating mail rule: %s (note: this feature requires a work/school account)", graphErrorMessage(err))
	}

	result := convertMailRule(created)
	return &result, nil
}

func (c *Client) DeleteMailRule(ctx context.Context, ruleID string) error {
	if err := validateID(ruleID, "rule ID"); err != nil {
		return err
	}
	err := c.inner.Me().MailFolders().ByMailFolderId("inbox").MessageRules().ByMessageRuleId(ruleID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting mail rule: %s", graphErrorMessage(err))
	}
	return nil
}

func convertMailRule(r models.MessageRuleable) MailRule {
	rule := MailRule{}
	if r.GetId() != nil {
		rule.ID = *r.GetId()
	}
	if r.GetDisplayName() != nil {
		rule.DisplayName = *r.GetDisplayName()
	}
	if r.GetSequence() != nil {
		rule.Sequence = *r.GetSequence()
	}
	if r.GetIsEnabled() != nil {
		rule.IsEnabled = *r.GetIsEnabled()
	}

	// Summarize conditions
	var conds []string
	if c := r.GetConditions(); c != nil {
		if addrs := c.GetFromAddresses(); len(addrs) > 0 {
			var emails []string
			for _, a := range addrs {
				if a.GetEmailAddress() != nil && a.GetEmailAddress().GetAddress() != nil {
					emails = append(emails, *a.GetEmailAddress().GetAddress())
				}
			}
			if len(emails) > 0 {
				conds = append(conds, "from:"+strings.Join(emails, ","))
			}
		}
		if sc := c.GetSubjectContains(); len(sc) > 0 {
			conds = append(conds, "subject:"+strings.Join(sc, ","))
		}
		if c.GetHasAttachments() != nil && *c.GetHasAttachments() {
			conds = append(conds, "has-attachment")
		}
	}
	rule.Conditions = strings.Join(conds, "; ")

	// Summarize actions
	var acts []string
	if a := r.GetActions(); a != nil {
		if a.GetMoveToFolder() != nil && *a.GetMoveToFolder() != "" {
			acts = append(acts, "move:"+*a.GetMoveToFolder())
		}
		if a.GetDelete() != nil && *a.GetDelete() {
			acts = append(acts, "delete")
		}
		if a.GetMarkAsRead() != nil && *a.GetMarkAsRead() {
			acts = append(acts, "mark-read")
		}
		if a.GetMarkImportance() != nil {
			acts = append(acts, "importance:"+a.GetMarkImportance().String())
		}
		if fwd := a.GetForwardTo(); len(fwd) > 0 {
			var emails []string
			for _, r := range fwd {
				if r.GetEmailAddress() != nil && r.GetEmailAddress().GetAddress() != nil {
					emails = append(emails, *r.GetEmailAddress().GetAddress())
				}
			}
			if len(emails) > 0 {
				acts = append(acts, "forward:"+strings.Join(emails, ","))
			}
		}
		if a.GetStopProcessingRules() != nil && *a.GetStopProcessingRules() {
			acts = append(acts, "stop-processing")
		}
	}
	rule.Actions = strings.Join(acts, "; ")

	return rule
}
