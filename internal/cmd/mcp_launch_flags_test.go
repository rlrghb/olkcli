package cmd

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Every field of RootFlags must be accounted for in one of these buckets. The
// bug this guards against is a global flag that is simply forgotten: a tool call
// re-parses a fresh CLI from an argv that carries none of them, so a flag nobody
// deliberately routed is silently dropped and the operator's command line has no
// effect. Classifying each field forces that decision to be made once, in the
// open, rather than discovered later from a server that ignored --dry-run.
var (
	// Restored from the launch values by applyLaunchEnv.
	launchCarriedFlags = map[string]string{
		"Account":      "which identity every call runs as",
		"Timeout":      "per-call ceiling the operator chose",
		"TimeZone":     "display zone for times in tool output",
		"Select":       "field projection the operator narrowed to",
		"ImmutableIDs": "ID stability contract the operator opted into",
		"ResultsOnly":  "envelope suppression, tightening only",
		"Concise":      "size reduction, tightening only",
		"Verbose":      "Graph request logging the operator asked for; the handler redacts Authorization",
		"DryRun":       "describe mutations instead of making them",
		"NoWrite":      "capability guard, tightening only",
		"NoSend":       "capability guard, tightening only",
	}

	// Set by the MCP layer itself, whatever the parse or the launch line said.
	launchForcedFlags = map[string]string{
		"JSON":          "buildArgv appends --json so output is machine-readable",
		"NoInput":       "forced on: a tool call has no terminal to prompt at",
		"WrapUntrusted": "forced on: external free text must be marked as data",
	}

	// Read once when the registry is built; a per-call value would be inert.
	launchRegistrationFlags = map[string]string{
		"EnableCommands":      "filters which tools are registered",
		"EnableCommandsExact": "filters which tools are registered",
		"DisableCommands":     "filters which tools are registered",
	}

	// Resolved by buildArgv per call rather than copied wholesale.
	launchArgvFlags = map[string]string{
		"Mailbox": "buildArgv picks the launch default or a permitted per-call choice",
		"Force":   "buildArgv supplies --force for destructive tools only",
	}

	// Deliberately not carried.
	launchIgnoredFlags = map[string]string{
		"Plain": "the forced --json outranks it in NewPrinter, so it cannot take effect",
		"Color": "output is captured, never a terminal, so colour never applies",
	}
)

func TestRootFlagsAreAllClassifiedForMCP(t *testing.T) {
	buckets := []struct {
		name   string
		fields map[string]string
	}{
		{"carried", launchCarriedFlags},
		{"forced", launchForcedFlags},
		{"registration-only", launchRegistrationFlags},
		{"argv-resolved", launchArgvFlags},
		{"intentionally ignored", launchIgnoredFlags},
	}

	where := map[string][]string{}
	for _, b := range buckets {
		for field := range b.fields {
			where[field] = append(where[field], b.name)
		}
	}

	typ := reflect.TypeOf(RootFlags{})
	declared := map[string]bool{}
	for i := range typ.NumField() {
		field := typ.Field(i).Name
		declared[field] = true
		switch len(where[field]) {
		case 1:
		case 0:
			t.Errorf("RootFlags.%s is not classified: decide whether an MCP tool call should "+
				"carry it, force it, resolve it per call, or ignore it, then add it to the "+
				"matching map in this file", field)
		default:
			sort.Strings(where[field])
			t.Errorf("RootFlags.%s is classified %d times (%s); it must appear exactly once",
				field, len(where[field]), strings.Join(where[field], ", "))
		}
	}

	for field := range where {
		if !declared[field] {
			t.Errorf("%q is classified but is not a field of RootFlags; remove the stale entry", field)
		}
	}
}

// The classification above is only a promise; this checks applyLaunchEnv keeps
// it. Every carried field is set to a distinctive launch value against a CLI
// holding a different one, which is what an ambient OLK_* variable surviving the
// per-call parse looks like.
func TestApplyLaunchEnv_CarriesEveryClassifiedField(t *testing.T) {
	env := &callEnv{
		account:      "svc@example.com",
		timeout:      120,
		timezone:     "America/New_York",
		selectFields: SelectFields{Value: "id,subject", Set: true},
		immutableIDs: true,
		verbose:      true,
		resultsOnly:  true,
		concise:      true,
		dryRun:       true,
		noWrite:      true,
		noSend:       true,
	}

	ambient := &CLI{}
	ambient.Account = "personal@example.com"
	ambient.Timeout = 60
	ambient.TimeZone = "UTC"
	ambient.Select = SelectFields{Value: "id", Set: true}
	applyLaunchEnv(ambient, env)

	checks := map[string]struct{ got, want any }{
		"Account":      {ambient.Account, "svc@example.com"},
		"Timeout":      {ambient.Timeout, 120},
		"TimeZone":     {ambient.TimeZone, "America/New_York"},
		"Select":       {ambient.Select.Value, "id,subject"},
		"ImmutableIDs": {ambient.ImmutableIDs, true},
		"Verbose":      {ambient.Verbose, true},
		"ResultsOnly":  {ambient.ResultsOnly, true},
		"Concise":      {ambient.Concise, true},
		"DryRun":       {ambient.DryRun, true},
		"NoWrite":      {ambient.NoWrite, true},
		"NoSend":       {ambient.NoSend, true},
	}
	for field := range launchCarriedFlags {
		c, ok := checks[field]
		if !ok {
			t.Errorf("%s is classified as carried but this test does not exercise it", field)
			continue
		}
		if c.got != c.want {
			t.Errorf("after applyLaunchEnv, %s = %v, want the launch value %v", field, c.got, c.want)
		}
	}
}

// The tightening fields must never be lowered by a call, and the overwriting
// fields must leave an ambient value alone when the server set nothing.
func TestApplyLaunchEnv_TighteningAndAbsence(t *testing.T) {
	tightened := &CLI{}
	tightened.DryRun = true
	tightened.Concise = true
	tightened.ResultsOnly = true
	tightened.ImmutableIDs = true
	tightened.NoWrite = true
	tightened.NoSend = true
	applyLaunchEnv(tightened, &callEnv{})
	if !tightened.DryRun || !tightened.Concise || !tightened.ResultsOnly ||
		!tightened.ImmutableIDs || !tightened.NoWrite || !tightened.NoSend {
		t.Error("a launch env that sets nothing must not lower a value the call already had")
	}

	envOnly := &CLI{}
	envOnly.Account = "personal@example.com"
	envOnly.TimeZone = "UTC"
	envOnly.Select = SelectFields{Value: "id", Set: true}
	applyLaunchEnv(envOnly, &callEnv{})
	if envOnly.Account != "personal@example.com" || envOnly.TimeZone != "UTC" ||
		envOnly.Select.Value != "id" {
		t.Error("with nothing set at launch, the ambient environment must still apply")
	}
}

// The two prior tests start from a callEnv, which is one step past where the
// original bug lived: the flag was lost between the operator's command line and
// the callEnv, not after it. This drives the whole chain — RootFlags through
// launchEnv and applyLaunchEnv — so dropping an assignment at either boundary
// fails here.
func TestLaunchFlagsSurviveFromCommandLineToCall(t *testing.T) {
	flags := &RootFlags{
		Account:      "svc@example.com",
		Timeout:      120,
		TimeZone:     "America/New_York",
		Select:       SelectFields{Value: "id,subject", Set: true},
		ImmutableIDs: true,
		ResultsOnly:  true,
		Concise:      true,
		DryRun:       true,
		Verbose:      true,
		NoWrite:      true,
		NoSend:       true,
	}

	env := launchEnv(flags, "team@example.com", []string{"team@example.com"})
	if env.mailbox != "team@example.com" {
		t.Fatalf("launch mailbox = %q, want team@example.com", env.mailbox)
	}

	cli := &CLI{}
	applyLaunchEnv(cli, &env)

	got := map[string]any{
		"Account":      cli.Account,
		"Timeout":      cli.Timeout,
		"TimeZone":     cli.TimeZone,
		"Select":       cli.Select.Value,
		"ImmutableIDs": cli.ImmutableIDs,
		"ResultsOnly":  cli.ResultsOnly,
		"Concise":      cli.Concise,
		"DryRun":       cli.DryRun,
		"Verbose":      cli.Verbose,
		"NoWrite":      cli.NoWrite,
		"NoSend":       cli.NoSend,
	}
	want := map[string]any{
		"Account":      "svc@example.com",
		"Timeout":      120,
		"TimeZone":     "America/New_York",
		"Select":       "id,subject",
		"ImmutableIDs": true,
		"ResultsOnly":  true,
		"Concise":      true,
		"DryRun":       true,
		"Verbose":      true,
		"NoWrite":      true,
		"NoSend":       true,
	}
	for field := range launchCarriedFlags {
		if _, ok := got[field]; !ok {
			t.Errorf("%s is classified as carried but this test does not follow it "+
				"from the command line; add it here and to launchEnv", field)
			continue
		}
		if got[field] != want[field] {
			t.Errorf("%s reached the call as %v, want the operator's %v", field, got[field], want[field])
		}
	}
}

// The finding this closes: a server started with --dry-run re-parsed an argv
// that never mentioned it, so every mutating tool ran for real. Driving the same
// path a tool call takes is what proves it now survives.
func TestPrepareCall_LaunchDryRunReachesTheCommand(t *testing.T) {
	b := &toolBinding{
		name: "mail_send",
		path: []string{"mail", "send"},
		node: leafByPath(t, "mail", "send"),
		tier: tierSend,
		env:  callEnv{dryRun: true},
	}
	argv, err := buildArgv(b, map[string]any{
		"to":      []any{"person@example.com"},
		"subject": "Preview",
		"body":    "Body",
	})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	for _, tok := range argv {
		if tok == "--dry-run" {
			t.Fatal("buildArgv should not add --dry-run itself; the launch env carries it")
		}
	}

	cli, kctx, err := prepareCall(argv, &b.env)
	if err != nil {
		t.Fatalf("prepareCall: %v", err)
	}
	if kctx == nil {
		t.Fatal("prepareCall returned no kong context")
	}
	if !cli.DryRun {
		t.Fatal("a server started with --dry-run must not run a mutating tool for real")
	}
	if !cli.NoInput || !cli.WrapUntrusted {
		t.Error("prepareCall must force --no-input and --wrap-untrusted")
	}
}
