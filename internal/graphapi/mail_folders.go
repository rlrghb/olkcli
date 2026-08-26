package graphapi

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

const (
	mailFolderPageSize         int32 = 100
	mailFolderDisplayNameField       = "displayName"
	mailFolderParentIDField          = "parentFolderId"
)

var mailFolderSelectFields = []string{
	"id",
	mailFolderDisplayNameField,
	"totalItemCount",
	"unreadItemCount",
	"childFolderCount",
	mailFolderParentIDField,
}

type mailFolderPage struct {
	values   []models.MailFolderable
	nextLink string
}

// ListMailFolders returns every visible folder in the target mailbox, or in
// the signed-in user's mailbox when target is empty. Graph's /mailFolders
// collection contains only the folders immediately below the mailbox root, so
// each folder that reports children is traversed through /childFolders.
func (c *Client) ListMailFolders(ctx context.Context, target string) ([]MailFolder, error) {
	rootFolders, err := c.listMailFolderCollection(ctx, target, "")
	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}

	result := make([]MailFolder, 0, len(rootFolders))
	queue := append([]models.MailFolderable(nil), rootFolders...)
	seen := make(map[string]struct{}, len(rootFolders))
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value := queue[0]
		queue = queue[1:]
		if value == nil || value.GetId() == nil || *value.GetId() == "" {
			return nil, fmt.Errorf("folder collection contains a folder without an ID")
		}
		id := *value.GetId()
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("folder traversal contains duplicate folder ID %q", id)
		}
		seen[id] = struct{}{}
		result = append(result, convertMailFolder(value))

		if value.GetChildFolderCount() == nil || *value.GetChildFolderCount() == 0 {
			continue
		}
		children, err := c.listMailFolderCollection(ctx, target, id)
		if err != nil {
			return nil, fmt.Errorf("listing child folders under folder %q: %w", id, err)
		}
		queue = append(queue, children...)
	}
	return result, nil
}

// ResolveMailFolderPath turns a display-name path such as Inbox/2026 into the
// ID Graph requires. A slash-bearing value whose first component does not name
// a top-level folder is left unchanged so existing Graph IDs containing slash
// characters remain valid inputs.
func (c *Client) ResolveMailFolderPath(ctx context.Context, target, reference string) (string, error) {
	if !strings.Contains(reference, "/") {
		return reference, nil
	}
	parts := strings.Split(reference, "/")
	rootFolders, err := c.listMailFolderCollection(ctx, target, "")
	if err != nil {
		return "", fmt.Errorf("resolving mail folder path %q: %w", reference, err)
	}

	current, err := matchMailFolderName(rootFolders, parts[0])
	if err != nil {
		return "", fmt.Errorf("resolving mail folder path %q: %w", reference, err)
	}
	if current == nil {
		return reference, nil
	}
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			return "", fmt.Errorf("mail folder path %q contains an empty component", reference)
		}
		children, err := c.listMailFolderCollection(ctx, target, derefStr(current.GetId()))
		if err != nil {
			return "", fmt.Errorf("resolving mail folder path %q below %q: %w", reference, parts[i-1], err)
		}
		next, err := matchMailFolderName(children, parts[i])
		if err != nil {
			return "", fmt.Errorf("resolving mail folder path %q: %w", reference, err)
		}
		if next == nil {
			return "", fmt.Errorf("mail folder path %q: component %q not found below %q", reference, parts[i], parts[i-1])
		}
		current = next
	}
	return derefStr(current.GetId()), nil
}

func matchMailFolderName(folders []models.MailFolderable, name string) (models.MailFolderable, error) {
	if name == "" {
		return nil, nil
	}
	exact := make([]models.MailFolderable, 0, 1)
	folded := make([]models.MailFolderable, 0, 1)
	for _, folder := range folders {
		if folder == nil || folder.GetDisplayName() == nil {
			continue
		}
		displayName := *folder.GetDisplayName()
		if displayName == name {
			exact = append(exact, folder)
		} else if strings.EqualFold(displayName, name) {
			folded = append(folded, folder)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = folded
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("folder name %q is ambiguous; use a folder ID", name)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return matches[0], nil
}

func (c *Client) listMailFolderCollection(ctx context.Context, target, parentID string) ([]models.MailFolderable, error) {
	first := func(ctx context.Context) (mailFolderPage, error) {
		top := mailFolderPageSize
		if parentID == "" {
			response, err := c.targetUser(target).MailFolders().Get(ctx, &users.ItemMailFoldersRequestBuilderGetRequestConfiguration{
				QueryParameters: &users.ItemMailFoldersRequestBuilderGetQueryParameters{
					Top:    &top,
					Select: mailFolderSelectFields,
				},
			})
			return mailFolderPageFrom(response, err)
		}
		response, err := c.targetUser(target).MailFolders().ByMailFolderId(parentID).ChildFolders().Get(ctx, &users.ItemMailFoldersItemChildFoldersRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemMailFoldersItemChildFoldersRequestBuilderGetQueryParameters{
				Top:    &top,
				Select: mailFolderSelectFields,
			},
		})
		return mailFolderPageFrom(response, err)
	}

	next := func(ctx context.Context, nextLink string) (mailFolderPage, error) {
		collection := "mailFolders"
		if parentID != "" {
			collection += "/" + url.PathEscape(parentID) + "/childFolders"
		}
		if err := validateGraphContinuation(nextLink, graphContinuationScope{
			host:           defaultGraphAPIHost,
			collectionPath: graphUserCollectionPath(target, collection),
		}); err != nil {
			return mailFolderPage{}, err
		}
		if parentID == "" {
			response, err := users.NewItemMailFoldersRequestBuilder(nextLink, c.inner.GetAdapter()).Get(ctx, nil)
			return mailFolderPageFrom(response, err)
		}
		response, err := users.NewItemMailFoldersItemChildFoldersRequestBuilder(nextLink, c.inner.GetAdapter()).Get(ctx, nil)
		return mailFolderPageFrom(response, err)
	}

	page, err := first(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]models.MailFolderable, 0, len(page.values))
	seenIDs := make(map[string]struct{})
	seenLinks := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(page.values) == 0 && page.nextLink != "" {
			return nil, fmt.Errorf("folder continuation made no progress")
		}
		for _, folder := range page.values {
			if folder == nil || folder.GetId() == nil || *folder.GetId() == "" {
				return nil, fmt.Errorf("folder collection contains a folder without an ID")
			}
			id := *folder.GetId()
			if _, exists := seenIDs[id]; exists {
				return nil, fmt.Errorf("folder collection contains duplicate folder ID %q", id)
			}
			seenIDs[id] = struct{}{}
			result = append(result, folder)
		}
		if page.nextLink == "" {
			return result, nil
		}
		if _, exists := seenLinks[page.nextLink]; exists {
			return nil, fmt.Errorf("folder continuation repeated a previous URL")
		}
		seenLinks[page.nextLink] = struct{}{}
		page, err = next(ctx, page.nextLink)
		if err != nil {
			return nil, err
		}
	}
}

func mailFolderPageFrom(response models.MailFolderCollectionResponseable, err error) (mailFolderPage, error) {
	if err != nil {
		return mailFolderPage{}, err
	}
	if response == nil {
		return mailFolderPage{}, errNilMailFolderResponse
	}
	return mailFolderPage{
		values:   response.GetValue(),
		nextLink: derefStr(response.GetOdataNextLink()),
	}, nil
}
