package cmd

import "testing"

func TestBuildArgv_AlwaysAppendsJSON(t *testing.T) {
	b := testBinding(t, "mail", "list")
	argv, err := buildArgv(b, map[string]any{})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if !contains(argv, "--json") {
		t.Errorf("argv %v missing --json", argv)
	}
	if argv[0] != "mail" || argv[1] != "list" {
		t.Errorf("argv should start with command path, got %v", argv)
	}
}

func TestBuildArgv_Scalars(t *testing.T) {
	b := testBinding(t, "mail", "list")
	// JSON numbers decode to float64.
	argv, err := buildArgv(b, map[string]any{"top": float64(10), "from": "a@b.com", "unread": true})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	assertAdjacent(t, argv, "--top", "10")
	assertAdjacent(t, argv, "--from", "a@b.com")
	if !contains(argv, "--unread") {
		t.Errorf("argv %v missing --unread", argv)
	}
}

func TestBuildArgv_BoolFalseOmitted(t *testing.T) {
	b := testBinding(t, "mail", "list")
	argv, err := buildArgv(b, map[string]any{"unread": false})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	if contains(argv, "--unread") {
		t.Errorf("false bool should be omitted, got %v", argv)
	}
}

func TestBuildArgv_SliceRepeats(t *testing.T) {
	b := testBinding(t, "mail", "send")
	argv, err := buildArgv(b, map[string]any{
		"to":      []any{"a@b.com", "c@d.com"},
		"subject": "Hi",
	})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	count := 0
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == "--to" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected --to repeated twice, got %d in %v", count, argv)
	}
}

func TestBuildArgv_PositionalsAfterSeparator(t *testing.T) {
	b := testBinding(t, "mail", "get")
	argv, err := buildArgv(b, map[string]any{"id": "AAMkABC"})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	sep := indexOf(argv, "--")
	if sep < 0 {
		t.Fatalf("expected -- separator in %v", argv)
	}
	// --json must come before the separator.
	if j := indexOf(argv, "--json"); j < 0 || j > sep {
		t.Errorf("--json (%d) must precede -- (%d) in %v", j, sep, argv)
	}
	// the positional value must come after the separator.
	if v := indexOf(argv, "AAMkABC"); v < sep {
		t.Errorf("positional should follow --, got %v", argv)
	}
}

func TestBuildArgv_MissingRequiredPositional(t *testing.T) {
	b := testBinding(t, "mail", "get")
	if _, err := buildArgv(b, map[string]any{}); err == nil {
		t.Error("expected error for missing required positional, got nil")
	}
}

func TestSprintArg(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{float64(25), "25"},
		{float64(1000000), "1000000"},
		{float64(1.5), "1.5"},
		{true, "true"},
		{false, "false"},
		{"hello", "hello"},
	}
	for _, tc := range cases {
		if got := sprintArg(tc.in); got != tc.want {
			t.Errorf("sprintArg(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func assertAdjacent(t *testing.T, argv []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == flag {
			if argv[i+1] == value {
				return
			}
			t.Errorf("%s followed by %q, want %q (argv %v)", flag, argv[i+1], value, argv)
			return
		}
	}
	t.Errorf("flag %s not found in %v", flag, argv)
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
