package graphapi

import "testing"

func TestValidateDeltaToken(t *testing.T) {
	ok := []string{
		"https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messages/delta?$deltatoken=abc",
		"https://graph.microsoft.us/v1.0/me/contacts/delta?$skiptoken=xyz",
		"https://microsoftgraph.chinacloudapi.cn/v1.0/me/calendarView/delta?$deltatoken=q",
	}
	for _, u := range ok {
		if err := validateDeltaToken(u); err != nil {
			t.Errorf("expected %q to be accepted, got %v", u, err)
		}
	}
	bad := []string{
		"http://graph.microsoft.com/v1.0/me/messages/delta",        // non-https
		"https://evil.example.com/v1.0/me/messages/delta?$token=x", // untrusted host
		"https://graph.microsoft.com.evil.com/delta",               // suffix trick
		"::not a url", // unparseable
	}
	for _, u := range bad {
		if err := validateDeltaToken(u); err == nil {
			t.Errorf("expected %q to be rejected", u)
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
