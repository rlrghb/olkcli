package graphapi

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
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
	return collectMessagePagesMode(ctx, limit, limit, true, first, next)
}

// collectAllMessagePages consumes continuation links until the provider
// returns a terminal page. pageSize bounds each request, not the total result.
func collectAllMessagePages(
	ctx context.Context,
	pageSize int32,
	first func(context.Context, int32) (messagePage, error),
	next func(context.Context, string, int32) (messagePage, error),
) ([]models.Messageable, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("message page size must be positive")
	}
	return collectMessagePagesMode(ctx, 0, pageSize, false, first, next)
}

func collectMessagePagesMode(
	ctx context.Context,
	limit int32,
	pageSize int32,
	bounded bool,
	first func(context.Context, int32) (messagePage, error),
	next func(context.Context, string, int32) (messagePage, error),
) ([]models.Messageable, error) {
	page, err := first(ctx, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]models.Messageable, 0)
	seenIDs := make(map[string]struct{})
	seenLinks := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(page.Values) == 0 && page.NextLink != "" {
			return nil, fmt.Errorf("message continuation made no progress")
		}

		for _, message := range page.Values {
			if bounded && int32(len(result)) == limit {
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

		if (bounded && int32(len(result)) == limit) || page.NextLink == "" {
			return result, nil
		}
		if _, exists := seenLinks[page.NextLink]; exists {
			return nil, fmt.Errorf("message continuation repeated a previous URL")
		}
		seenLinks[page.NextLink] = struct{}{}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nextTop := pageSize
		if bounded {
			nextTop = limit - int32(len(result))
		}
		page, err = next(ctx, page.NextLink, nextTop)
		if err != nil {
			return nil, err
		}
	}
}

func (c *Client) collectUserMessagePages(
	ctx context.Context,
	target string,
	pageSize int32,
	complete bool,
	query *users.ItemMessagesRequestBuilderGetQueryParameters,
) ([]models.Messageable, error) {
	first := func(ctx context.Context, top int32) (messagePage, error) {
		query.Top = &top
		response, err := c.targetUser(target).Messages().Get(ctx, &users.ItemMessagesRequestBuilderGetRequestConfiguration{
			Headers:         c.messageIDHeaders(nil),
			QueryParameters: query,
		})
		if err != nil {
			return messagePage{}, err
		}
		if response == nil {
			return messagePage{}, errNilMessageResponse
		}
		return messagePage{Values: response.GetValue(), NextLink: derefStr(response.GetOdataNextLink())}, nil
	}
	next := func(ctx context.Context, nextLink string, _ int32) (messagePage, error) {
		if err := validateGraphContinuation(nextLink, graphContinuationScope{
			host:           defaultGraphAPIHost,
			collectionPath: graphUserCollectionPath(target, "messages"),
		}); err != nil {
			return messagePage{}, err
		}
		response, err := users.NewItemMessagesRequestBuilder(nextLink, c.inner.GetAdapter()).Get(ctx, &users.ItemMessagesRequestBuilderGetRequestConfiguration{
			Headers: c.messageIDHeaders(nil),
		})
		if err != nil {
			return messagePage{}, err
		}
		if response == nil {
			return messagePage{}, errNilMessageResponse
		}
		return messagePage{Values: response.GetValue(), NextLink: derefStr(response.GetOdataNextLink())}, nil
	}
	if complete {
		return collectAllMessagePages(ctx, pageSize, first, next)
	}
	return collectMessagePages(ctx, pageSize, first, next)
}

const defaultGraphAPIHost = "graph.microsoft.com"

// graphContinuationScope limits a Graph continuation to one host and
// collection. A continuation is replayed through the authenticated SDK
// adapter, so both must be verified before the request.
type graphContinuationScope struct {
	host           string
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
	expectedHost := strings.ToLower(expected.host)
	if expectedHost == "" || !graphAPIHosts[expectedHost] {
		return fmt.Errorf("refusing Graph continuation with invalid expected host")
	}
	if actualHost := strings.ToLower(u.Hostname()); actualHost != expectedHost {
		return fmt.Errorf("refusing Graph continuation for unexpected host %q", u.Hostname())
	}
	if !sameGraphCollection(u.EscapedPath(), expected.collectionPath) {
		return fmt.Errorf("refusing Graph continuation outside expected collection")
	}
	return nil
}

type graphCollectionOperation uint8

const (
	graphMessageCollection graphCollectionOperation = iota + 1
	graphMessageDeltaCollection
	graphCalendarViewDeltaCollection
	graphContactsDeltaCollection
)

type graphCollectionRoute struct {
	isMe      bool
	userID    string
	folderID  string
	operation graphCollectionOperation
}

func sameGraphCollection(actualPath, expectedPath string) bool {
	actual, ok := parseGraphCollectionRoute(actualPath)
	if !ok {
		return false
	}
	expected, ok := parseGraphCollectionRoute(expectedPath)
	return ok && actual == expected
}

func parseGraphCollectionRoute(path string) (graphCollectionRoute, bool) {
	if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return graphCollectionRoute{}, false
	}
	segments := strings.Split(path[1:], "/")
	if len(segments) < 3 || !strings.EqualFold(segments[0], "v1.0") {
		return graphCollectionRoute{}, false
	}

	route := graphCollectionRoute{}
	var collection []string
	switch {
	case strings.EqualFold(segments[1], "me"):
		route.isMe = true
		collection = segments[2:]
	case strings.EqualFold(segments[1], "users") && len(segments) >= 4:
		userID, ok := graphPathSegment(segments[2])
		if !ok || userID == "" {
			return graphCollectionRoute{}, false
		}
		route.userID = userID
		collection = segments[3:]
	default:
		return graphCollectionRoute{}, false
	}
	if len(collection) == 1 && strings.EqualFold(collection[0], "messages") {
		route.operation = graphMessageCollection
		return route, true
	}

	if folderID, consumed, ok := graphMailFolderRoute(collection); ok {
		route.folderID = folderID
		collection = collection[consumed:]
		if len(collection) == 1 && strings.EqualFold(collection[0], "messages") {
			route.operation = graphMessageCollection
			return route, true
		}
		if len(collection) == 2 && strings.EqualFold(collection[0], "messages") && strings.EqualFold(collection[1], "delta") {
			route.operation = graphMessageDeltaCollection
			return route, true
		}
		return graphCollectionRoute{}, false
	}

	if len(collection) == 2 && strings.EqualFold(collection[0], "calendarView") && strings.EqualFold(collection[1], "delta") {
		route.operation = graphCalendarViewDeltaCollection
		return route, true
	}
	if len(collection) == 2 && strings.EqualFold(collection[0], "contacts") && strings.EqualFold(collection[1], "delta") {
		route.operation = graphContactsDeltaCollection
		return route, true
	}
	return graphCollectionRoute{}, false
}

func graphMailFolderRoute(segments []string) (folderID string, consumed int, ok bool) {
	if len(segments) == 0 {
		return "", 0, false
	}
	folder, ok := graphPathSegment(segments[0])
	if !ok {
		return "", 0, false
	}
	if strings.EqualFold(folder, "mailfolders") {
		if len(segments) < 2 {
			return "", 0, false
		}
		folderID, ok := graphPathSegment(segments[1])
		return folderID, 2, ok && folderID != ""
	}

	const alternateKeyPrefix = "mailfolders('"
	if len(folder) <= len(alternateKeyPrefix) || !strings.EqualFold(folder[:len(alternateKeyPrefix)], alternateKeyPrefix) || !strings.HasSuffix(folder, "')") {
		return "", 0, false
	}
	folderID, ok = graphODataString(folder[len(alternateKeyPrefix) : len(folder)-2])
	return folderID, 1, ok && folderID != ""
}

func graphPathSegment(raw string) (string, bool) {
	value, err := url.PathUnescape(raw)
	return value, err == nil
}

func graphODataString(raw string) (string, bool) {
	var value strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\'' {
			value.WriteByte(raw[i])
			continue
		}
		if i+1 >= len(raw) || raw[i+1] != '\'' {
			return "", false
		}
		value.WriteByte('\'')
		i++
	}
	return value.String(), true
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
		host:           defaultGraphAPIHost,
		collectionPath: graphUserCollectionPath(target, "mailFolders/"+url.PathEscape(folderID)+"/messages/delta"),
	}
}

func calendarViewDeltaScope(target string) graphContinuationScope {
	return graphContinuationScope{host: defaultGraphAPIHost, collectionPath: graphUserCollectionPath(target, "calendarView/delta")}
}

func contactsDeltaScope(target string) graphContinuationScope {
	return graphContinuationScope{host: defaultGraphAPIHost, collectionPath: graphUserCollectionPath(target, "contacts/delta")}
}
