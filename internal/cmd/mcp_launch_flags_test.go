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
		"ResultsOnly":  "envelope suppression the operator asked for",
		"Concise":      "size reduction; the only carried flag a call may also set, so it acts as a floor",
		"Verbose":      "Graph request logging the operator asked for; the handler redacts Authorization",
		"DryRun":       "describe mutations instead of making them",
		"NoWrite":      "capability guard; registration filtered the tool list on this value",
		"NoSend":       "capability guard; registration filtered the tool list on this value",
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

// The launch snapshot replaces rather than supplements. An empty or false launch
// value is the operator's answer, so it must clear whatever the per-call parse
// picked up from the environment — which is where the ambient OLK_* variables
// re-enter. --concise is the exception, being the one flag a call may set itself.
func TestApplyLaunchEnv_SnapshotReplacesAmbientValues(t *testing.T) {
	ambient := &CLI{}
	ambient.Account = "personal@example.com"
	ambient.Timeout = 30
	ambient.TimeZone = "UTC"
	ambient.Select = SelectFields{Value: "id", Set: true}
	ambient.Verbose = true
	ambient.ImmutableIDs = true
	ambient.ResultsOnly = true
	ambient.DryRun = true
	ambient.NoWrite = true
	ambient.NoSend = true

	applyLaunchEnv(ambient, &callEnv{})

	if ambient.Account != "" || ambient.TimeZone != "" || ambient.Select.Value != "" {
		t.Errorf("account=%q tz=%q select=%q, want all cleared by a launch line that named none",
			ambient.Account, ambient.TimeZone, ambient.Select.Value)
	}
	if ambient.Timeout != 0 {
		t.Errorf("timeout = %d, want the launch value; the caller applies the default", ambient.Timeout)
	}
	if ambient.Verbose || ambient.ImmutableIDs || ambient.ResultsOnly || ambient.DryRun ||
		ambient.NoWrite || ambient.NoSend {
		t.Error("a launch line that set none of these must not inherit them from the environment")
	}

	// --concise is the one a tool call may set for itself, so the launch value is
	// a floor rather than a replacement.
	perCall := &CLI{}
	perCall.Concise = true
	applyLaunchEnv(perCall, &callEnv{})
	if !perCall.Concise {
		t.Error("a call that asked to be concise must stay concise")
	}
	floor := &CLI{}
	applyLaunchEnv(floor, &callEnv{concise: true})
	if !floor.Concise {
		t.Error("a server started with --concise must apply it to a call that did not ask")
	}
}

// The same thing through a real parse, which is where the ambient variables
// actually arrive: kong re-reads every OLK_* on each tool call.
func TestPrepareCall_AmbientEnvironmentCannotOutrankTheLaunchLine(t *testing.T) {
	t.Setenv("OLK_ACCOUNT", "ambient@example.com")
	t.Setenv("OLK_MAILBOX", "ambient-box@example.com")
	t.Setenv("OLK_VERBOSE", "true")
	t.Setenv("OLK_DRY_RUN", "true")
	t.Setenv("OLK_NO_WRITE", "true")
	t.Setenv("OLK_NO_SEND", "true")

	b := &toolBinding{
		name: "mail_list",
		path: []string{"mail", "list"},
		node: leafByPath(t, "mail", "list"),
		tier: tierRead,
		env:  callEnv{}, // an operator who named nothing on the command line
	}
	argv, err := buildArgv(b, map[string]any{})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	cli, _, err := prepareCall(argv, &b.env)
	if err != nil {
		t.Fatalf("prepareCall: %v", err)
	}
	if cli.Account != "" {
		t.Errorf("account = %q; an ambient OLK_ACCOUNT must not outrank a launch line that named none", cli.Account)
	}
	if cli.Mailbox != "" {
		t.Errorf("mailbox = %q; an ambient OLK_MAILBOX must not redirect a call", cli.Mailbox)
	}
	if cli.Verbose {
		t.Error("an ambient OLK_VERBOSE must not enable logging the operator turned off")
	}
	if cli.DryRun {
		t.Error("an ambient OLK_DRY_RUN must not silently neuter every write")
	}
	// The guards matter most: registration filtered the tool list on the launch
	// values, so enforcement picking up different ones would disagree with what
	// the agent was told it could call.
	if cli.NoWrite || cli.NoSend {
		t.Error("ambient guard variables must not outrank a launch line that set neither")
	}

	// And the launch line still wins when it does name something.
	b.env = callEnv{account: "svc@example.com", mailbox: "team@example.com"}
	argv, err = buildArgv(b, map[string]any{})
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	cli, _, err = prepareCall(argv, &b.env)
	if err != nil {
		t.Fatalf("prepareCall: %v", err)
	}
	if cli.Account != "svc@example.com" || cli.Mailbox != "team@example.com" {
		t.Errorf("account=%q mailbox=%q, want the launch values", cli.Account, cli.Mailbox)
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
