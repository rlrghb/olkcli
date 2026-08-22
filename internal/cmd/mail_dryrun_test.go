package cmd

import (
	"net/http"
	"testing"
)

// TestDryRunPreviewsShowAllRecipientClasses pins the recipient block of the
// `mail send` and `mail drafts create` previews: every supplied class is
// listed, the optional ones disappear when unused, and no Graph request is
// made.
func TestDryRunPreviewsShowAllRecipientClasses(t *testing.T) {
	tests := []struct {
		name     string
		path     []string
		args     []string
		expected string
	}{
		{
			name: "send with every recipient class",
			path: []string{"mail", "send"},
			args: []string{
				"--to", "to-one@example.com,to-two@example.com",
				"--cc", "cc@example.com",
				"--bcc", "bcc@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would send email:\n" +
				"  From: your own mailbox\n" +
				"  To: to-one@example.com, to-two@example.com\n" +
				"  Cc: cc@example.com\n" +
				"  Bcc: bcc@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
		{
			name: "send without optional recipients",
			path: []string{"mail", "send"},
			args: []string{
				"--to", "to-one@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would send email:\n" +
				"  From: your own mailbox\n" +
				"  To: to-one@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
		{
			name: "send with cc only",
			path: []string{"mail", "send"},
			args: []string{
				"--to", "to-one@example.com",
				"--cc", "cc@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would send email:\n" +
				"  From: your own mailbox\n" +
				"  To: to-one@example.com\n" +
				"  Cc: cc@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
		{
			name: "draft with every recipient class",
			path: []string{"mail", "drafts", "create"},
			args: []string{
				"--to", "to-one@example.com,to-two@example.com",
				"--cc", "cc-one@example.com,cc-two@example.com",
				"--bcc", "bcc@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would create draft:\n" +
				"  In: your own mailbox\n" +
				"  To: to-one@example.com, to-two@example.com\n" +
				"  Cc: cc-one@example.com, cc-two@example.com\n" +
				"  Bcc: bcc@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
		{
			name: "draft without optional recipients",
			path: []string{"mail", "drafts", "create"},
			args: []string{
				"--to", "to-one@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would create draft:\n" +
				"  In: your own mailbox\n" +
				"  To: to-one@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
		{
			name: "draft with multiple bcc only",
			path: []string{"mail", "drafts", "create"},
			args: []string{
				"--to", "to-one@example.com",
				"--bcc", "bcc-one@example.com,bcc-two@example.com",
				"--subject", "Preview",
				"--body", "Body",
			},
			expected: "Would create draft:\n" +
				"  In: your own mailbox\n" +
				"  To: to-one@example.com\n" +
				"  Bcc: bcc-one@example.com, bcc-two@example.com\n" +
				"  Subject: Preview\n" +
				"  Body: Body\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output, calls, err := runMailCommand(
				t,
				tc.path,
				append([]string{"--dry-run"}, tc.args...),
				func(req *http.Request) *http.Response {
					t.Errorf("unexpected Graph request during dry run: %s %s", req.Method, req.URL.Path)
					return graphJSONResponse(req, `{}`)
				},
			)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if calls != 0 {
				t.Fatalf("Graph handler calls = %d, want 0", calls)
			}
			if output != tc.expected {
				t.Fatalf("preview =\n%q\nwant\n%q", output, tc.expected)
			}
		})
	}
}
