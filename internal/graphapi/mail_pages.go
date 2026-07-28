package graphapi

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// messagePage is one page of Graph messages and its opaque continuation URL.
type messagePage struct {
	Values   []models.Messageable
	NextLink string
}

// collectMessagePages gathers at most limit messages from successive Graph
// pages. It requests no more than the remaining result count on each call and
// rejects server responses that could loop or duplicate user-visible results.
func collectMessagePages(
	ctx context.Context,
	limit int32,
	first func(context.Context, int32) (messagePage, error),
	next func(context.Context, string, int32) (messagePage, error),
) ([]models.Messageable, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []models.Messageable{}, nil
	}

	page, err := first(ctx, limit)
	if err != nil {
		return nil, err
	}

	result := make([]models.Messageable, 0, limit)
	seenIDs := make(map[string]struct{}, limit)
	seenLinks := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(page.Values) == 0 && page.NextLink != "" {
			return nil, fmt.Errorf("message continuation made no progress")
		}

		for _, message := range page.Values {
			if int32(len(result)) == limit {
				return result, nil
			}
			if message == nil || message.GetId() == nil || *message.GetId() == "" {
				return nil, fmt.Errorf("message page contains a message without an ID")
			}
			id := *message.GetId()
			if _, exists := seenIDs[id]; exists {
				return nil, fmt.Errorf("message page contains duplicate message ID %q", id)
			}
			seenIDs[id] = struct{}{}
			result = append(result, message)
		}

		if int32(len(result)) == limit || page.NextLink == "" {
			return result, nil
		}
		if _, exists := seenLinks[page.NextLink]; exists {
			return nil, fmt.Errorf("message continuation repeated a previous URL")
		}
		seenLinks[page.NextLink] = struct{}{}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := limit - int32(len(result))
		page, err = next(ctx, page.NextLink, remaining)
		if err != nil {
			return nil, err
		}
	}
}

// graphContinuationScope limits a Graph continuation to one collection path.
// A continuation is replayed through the authenticated SDK adapter, so both
// its Graph host and operation path must be verified before the request.
type graphContinuationScope struct {
	collectionPath string
}

// validateGraphContinuation rejects a continuation URL that would replay an
// authenticated request outside its expected Graph collection.
func validateGraphContinuation(raw string, expected graphContinuationScope) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid Graph continuation")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("refusing non-HTTPS Graph continuation")
	}
	if u.User != nil {
		return fmt.Errorf("refusing Graph continuation with userinfo")
	}
	if u.Fragment != "" {
		return fmt.Errorf("refusing Graph continuation with fragment")
	}
	if port := u.Port(); port != "" && port != "443" {
		return fmt.Errorf("refusing Graph continuation on port %q", port)
	}
	if !graphAPIHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("refusing Graph continuation for untrusted host %q", u.Hostname())
	}
	if expected.collectionPath == "" || u.EscapedPath() != expected.collectionPath {
		return fmt.Errorf("refusing Graph continuation outside expected collection")
	}
	return nil
}

func graphUserCollectionPath(target, collection string) string {
	user := "me"
	if target != "" {
		user = "users/" + url.PathEscape(target)
	}
	return "/v1.0/" + user + "/" + collection
}

func mailMessagesDeltaScope(target, folderID string) graphContinuationScope {
	return graphContinuationScope{
		collectionPath: graphUserCollectionPath(target, "mailFolders/"+url.PathEscape(folderID)+"/messages/delta"),
	}
}

func calendarViewDeltaScope(target string) graphContinuationScope {
	return graphContinuationScope{collectionPath: graphUserCollectionPath(target, "calendarView/delta")}
}

func contactsDeltaScope(target string) graphContinuationScope {
	return graphContinuationScope{collectionPath: graphUserCollectionPath(target, "contacts/delta")}
}
