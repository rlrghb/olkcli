package cmd

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		path []string
		want cmdKind
	}{
		{"mail list is read", []string{"mail", "list"}, kindRead},
		{"mail get is read", []string{"mail", "get"}, kindRead},
		{"mail send is write", []string{"mail", "send"}, kindWrite},
		{"mail delete is destructive", []string{"mail", "delete"}, kindDestructive},
		{"nested categories delete is destructive", []string{"mail", "categories", "delete"}, kindDestructive},
		{"nested todo lists delete is destructive", []string{"todo", "lists", "delete"}, kindDestructive},
		{"drive rm is destructive", []string{"drive", "rm"}, kindDestructive},
		{"drive mv is write", []string{"drive", "mv"}, kindWrite},
		{"auth clean is destructive", []string{"auth", "clean"}, kindDestructive},
		{"auth logout is destructive", []string{"auth", "logout"}, kindDestructive},
		{"auth login is interactive", []string{"auth", "login"}, kindInteractive},
		{"auth list override is read", []string{"auth", "list"}, kindRead},
		{"whoami override is read", []string{"whoami"}, kindRead},
		{"version override is read", []string{"version"}, kindRead},
		{"config get override is read", []string{"config", "get"}, kindRead},
		{"config set is write", []string{"config", "set"}, kindWrite},
		{"calendar find-times is read", []string{"calendar", "find-times"}, kindRead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.path); got != tc.want {
				t.Errorf("classify(%v) = %d, want %d", tc.path, got, tc.want)
			}
		})
	}
}

func TestIncludeInProfile(t *testing.T) {
	cases := []struct {
		name      string
		profile   string
		kind      cmdKind
		wantSafe  bool
		wantFull  bool
		checkBoth bool
	}{
		{name: "read", kind: kindRead, wantSafe: true, wantFull: true, checkBoth: true},
		{name: "write", kind: kindWrite, wantSafe: true, wantFull: true, checkBoth: true},
		{name: "destructive", kind: kindDestructive, wantSafe: false, wantFull: true, checkBoth: true},
		{name: "interactive", kind: kindInteractive, wantSafe: false, wantFull: false, checkBoth: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := includeInProfile("safe", tc.kind); got != tc.wantSafe {
				t.Errorf("safe include of %s = %v, want %v", tc.name, got, tc.wantSafe)
			}
			if got := includeInProfile("full", tc.kind); got != tc.wantFull {
				t.Errorf("full include of %s = %v, want %v", tc.name, got, tc.wantFull)
			}
		})
	}
}
