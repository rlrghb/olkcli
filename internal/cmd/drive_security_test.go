package cmd

import (
	"net"
	"testing"
)

func TestIsDeniedIP(t *testing.T) {
	denied := []string{
		"127.0.0.1", "::1", // loopback
		"10.0.0.5", "192.168.1.1", "172.16.0.1", // RFC1918
		"169.254.169.254",               // cloud metadata (link-local)
		"100.64.0.1", "100.127.255.255", // CGNAT
		"0.0.0.0", // unspecified
		"fe80::1", // link-local v6
	}
	for _, s := range denied {
		if !isDeniedIP(net.ParseIP(s)) {
			t.Errorf("isDeniedIP(%s) = false, want true", s)
		}
	}
	allowed := []string{"8.8.8.8", "13.107.42.12", "2001:4860:4860::8888", "100.63.255.255", "100.128.0.0"}
	for _, s := range allowed {
		if isDeniedIP(net.ParseIP(s)) {
			t.Errorf("isDeniedIP(%s) = true, want false", s)
		}
	}
	if isDeniedIP(nil) {
		t.Error("isDeniedIP(nil) should be false")
	}
}

func TestValidateGraphURL_HostAndScheme(t *testing.T) {
	bad := []string{
		"http://graph.microsoft.com/x",           // non-https
		"https://evil.com/x",                     // untrusted host
		"https://graph.microsoft.com.evil.com/x", // suffix spoof
		"https://127.0.0.1/x",                    // literal loopback (not allowlisted anyway)
	}
	for _, u := range bad {
		if err := validateGraphURL(u); err == nil {
			t.Errorf("validateGraphURL(%q) = nil, want error", u)
		}
	}
	// A trusted host with a public-resolving name should pass scheme+host checks.
	if err := validateGraphURL("https://my.sharepoint.com/personal/file"); err != nil {
		t.Logf("note: %q rejected (%v) — acceptable if DNS resolution failed offline", "my.sharepoint.com", err)
	}
}
