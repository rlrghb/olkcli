package graphapi

import "testing"

func TestValidateDeltaContinuation(t *testing.T) {
	tests := []struct {
		url   string
		scope graphContinuationScope
	}{
		{
			url:   "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages/delta?$deltatoken=abc",
			scope: mailMessagesDeltaScope("", "inbox"),
		},
		{
			url:   "https://graph.microsoft.us/v1.0/me/contacts/delta?$skiptoken=xyz",
			scope: continuationScope("graph.microsoft.us", "/v1.0/me/contacts/delta"),
		},
		{
			url:   "https://microsoftgraph.chinacloudapi.cn/v1.0/me/calendarView/delta?$deltatoken=q",
			scope: continuationScope("microsoftgraph.chinacloudapi.cn", "/v1.0/me/calendarView/delta"),
		},
	}
	for _, tc := range tests {
		if err := validateGraphContinuation(tc.url, tc.scope); err != nil {
			t.Errorf("expected %q to be accepted, got %v", tc.url, err)
		}
	}
}

func TestIsRemoved(t *testing.T) {
	if isRemoved(nil) {
		t.Error("nil additionalData should not be removed")
	}
	if isRemoved(map[string]any{"foo": "bar"}) {
		t.Error("absent @removed should not be removed")
	}
	if !isRemoved(map[string]any{"@removed": map[string]any{"reason": "deleted"}}) {
		t.Error("@removed present should be removed")
	}
}
