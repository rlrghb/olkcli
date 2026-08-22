package graphapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// meBuilderPath is what the SDK's Me() builder emits before the Graph middleware
// rewrites it. The rewrite lives in the default pipeline (msgraph-sdk-go-core
// graph_client_factory.go ReplacementPairs), which the bare adapter these tests
// use does not install, so an own-mailbox request arrives here as
// /users/me-token-to-replace/... and reaches /me in production.
const meBuilderPath = "/v1.0/users/me-token-to-replace"

// The bug this pull request exists to fix was silent: a send with --mailbox set
// went out from the caller's own address and reported success, because the
// request still went to the caller's own mailbox. Guard and validation tests
// cannot see that — only the request path can, so these assert it directly, in
// both directions: a target must reach that mailbox, and no target must not
// reach some other one.
func TestDelegatedWritesAddressTheTargetMailbox(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
		call   func(*Client, context.Context, string) error
	}{
		{
			name:   "send from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/sendMail",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendMessage(ctx, target, &SendMessageOptions{
					Subject: "s", Body: "b", To: []string{"person@example.com"},
				})
			},
		},
		{
			name:   "send from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/sendMail",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendMessage(ctx, target, &SendMessageOptions{
					Subject: "s", Body: "b", To: []string{"person@example.com"},
				})
			},
		},
		{
			name:   "reply from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/reply",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", false)
			},
		},
		{
			name:   "reply-all from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/replyAll",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", true)
			},
		},
		{
			name:   "reply from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/reply",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", false)
			},
		},
		{
			name:   "forward from a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/forward",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ForwardMessage(ctx, target, "AAA", "", []string{"person@example.com"})
			},
		},
		{
			name:   "forward from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/forward",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ForwardMessage(ctx, target, "AAA", "", []string{"person@example.com"})
			},
		},
		{
			name:   "reply-all from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/replyAll",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.ReplyMessage(ctx, target, "AAA", "body", true)
			},
		},
		{
			name:   "create a draft in the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.CreateDraft(ctx, target, "s", "b", []string{"person@example.com"}, nil, nil, false)
				return err
			},
		},
		{
			name:   "send a draft from the caller's own mailbox",
			target: "",
			want:   meBuilderPath + "/messages/AAA/send",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendDraft(ctx, target, "AAA")
			},
		},
		{
			name:   "create a draft in a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages",
			call: func(c *Client, ctx context.Context, target string) error {
				_, err := c.CreateDraft(ctx, target, "s", "b", []string{"person@example.com"}, nil, nil, false)
				return err
			},
		},
		{
			name:   "send a draft that lives in a shared mailbox",
			target: "team@example.com",
			want:   "/v1.0/users/team@example.com/messages/AAA/send",
			call: func(c *Client, ctx context.Context, target string) error {
				return c.SendDraft(ctx, target, "AAA")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			calls := 0
			client := testGraphClient(t, func(req *http.Request) *http.Response {
				calls++
				got = req.URL.Path
				// Draft creation is the one call here that reads its response.
				if strings.HasSuffix(req.URL.Path, "/messages") {
					return graphJSONResponse(req, `{"id":"draft-id","subject":"s"}`)
				}
				return graphEmptyResponse(req)
			})

			if err := tc.call(client, context.Background(), tc.target); err != nil {
				t.Fatalf("call: %v", err)
			}
			if calls != 1 {
				t.Fatalf("Graph requests = %d, want 1", calls)
			}
			if got != tc.want {
				t.Errorf("request path = %q, want %q — a delegated write that lands on the "+
					"wrong path acts on the wrong mailbox and still reports success", got, tc.want)
			}
		})
	}
}

// A refused delegated send must arrive carrying Graph's error code.
//
// ODataError.Error() returns only the provider message, so wrapping one with %w
// renders prose that names neither the code nor the status. Everything that has
// to tell a refused send from a stale ID reads this string, so the code has to be
// in it.
func TestSharedMailboxErrorCarriesTheGraphCode(t *testing.T) {
	odataErr := odataerrors.NewODataError()
	main := odataerrors.NewMainError()
	code := "ErrorSendAsDenied"
	// Microsoft's documented message for this failure names neither the code nor
	// the phrase "access denied", which is exactly why the code must survive.
	message := "The user account which was used to submit this request does not have " +
		"the right to send mail on behalf of the specified sending account."
	main.SetCode(&code)
	main.SetMessage(&message)
	odataErr.SetErrorEscaped(main)
	odataErr.SetStatusCode(403)

	err := sharedMailboxError("sending message", "team@example.com", sendGrantHint, odataErr)
	for _, want := range []string{"ErrorSendAsDenied", "team@example.com", "Full Access"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error text does not mention %q:\n%s", want, err.Error())
		}
	}

	// The rendered text is only half of it. --json reads the code and the HTTP
	// status back off the original error, so it has to stay in the chain: a
	// helper that renders by formatting alone reduces every delegated failure to
	// CommandFailed with no status.
	if code, status := ErrorMetadata(err); code != "ErrorSendAsDenied" || status != 403 {
		t.Errorf("ErrorMetadata = (%q, %d), want (ErrorSendAsDenied, 403)", code, status)
	}
}

func TestSharedMailboxErrorOnlyAddsRelevantGuidance(t *testing.T) {
	cases := []struct {
		name       string
		code       string
		status     int
		message    string
		hint       string
		want       string
		wantAbsent string
	}{
		{
			name:       "stale reply ID gets mailbox ID guidance only",
			code:       "ErrorItemNotFound",
			status:     404,
			message:    "The item could not be found.",
			hint:       replyGrantHint,
			want:       replyIDHint,
			wantAbsent: "Mail.Send.Shared",
		},
		{
			name:       "throttle gets no permission guidance",
			code:       "TooManyRequests",
			status:     429,
			message:    "Too many requests.",
			hint:       sendGrantHint,
			wantAbsent: "Mail.Send.Shared",
		},
		{
			name:    "send refusal gets grant guidance",
			code:    "ErrorSendAsDenied",
			status:  403,
			message: "The user does not have the right to send as this mailbox.",
			hint:    sendGrantHint,
			want:    "Full Access",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			odataErr := odataerrors.NewODataError()
			main := odataerrors.NewMainError()
			main.SetCode(&tc.code)
			main.SetMessage(&tc.message)
			odataErr.SetErrorEscaped(main)
			odataErr.SetStatusCode(tc.status)

			err := sharedMailboxError("sending message", "team@example.com", tc.hint, odataErr)
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not mention %q:\n%s", tc.want, err)
			}
			if tc.wantAbsent != "" && strings.Contains(err.Error(), tc.wantAbsent) {
				t.Errorf("error unexpectedly contains %q:\n%s", tc.wantAbsent, err)
			}
		})
	}
}

// The same contract for the two older helpers, which had the same defect before
// graphError existed.
func TestGraphErrorHelpersStayUnwrappable(t *testing.T) {
	odataErr := odataerrors.NewODataError()
	main := odataerrors.NewMainError()
	code := "ErrorAccessDenied"
	message := "Access is denied. Check credentials and try again."
	main.SetCode(&code)
	main.SetMessage(&message)
	odataErr.SetErrorEscaped(main)
	odataErr.SetStatusCode(403)

	for name, err := range map[string]error{
		"enterpriseError":   enterpriseError("listing messages", odataErr),
		"scopeUpgradeError": scopeUpgradeError("listing messages", odataErr),
	} {
		if !strings.Contains(err.Error(), code) {
			t.Errorf("%s text does not name the code:\n%s", name, err.Error())
		}
		if gotCode, status := ErrorMetadata(err); gotCode != code || status != 403 {
			t.Errorf("%s: ErrorMetadata = (%q, %d), want (%s, 403)", name, gotCode, status, code)
		}
	}
}
