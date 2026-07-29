package graphapi

import abs "github.com/microsoft/kiota-abstractions-go"

const immutableIDPreference = `IdType="ImmutableId"`

func (c *Client) messageIDHeaders(
	headers *abs.RequestHeaders,
) *abs.RequestHeaders {
	if !c.immutableIDs {
		return headers
	}
	if headers == nil {
		headers = abs.NewRequestHeaders()
	}
	headers.Add(preferHeader, immutableIDPreference)
	return headers
}
