package cmd

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPCmd runs an MCP server that exposes olk commands as tools.
//
// Tool calls are executed in-process and serialized (one at a time), because
// capturing command output requires temporarily redirecting the process stdout.
type MCPCmd struct {
	Profile   string `help:"Tool exposure: 'safe' (reads + non-destructive writes) or 'full' (everything)" enum:"safe,full" default:"safe" env:"OLK_MCP_PROFILE"`
	HTTP      string `help:"Serve streamable HTTP at this address (e.g. :8765). Empty means stdio." name:"http" env:"OLK_MCP_HTTP"`
	HTTPToken string `help:"Bearer token required of HTTP clients (prefer the OLK_MCP_TOKEN env var)" name:"http-token" env:"OLK_MCP_TOKEN"`
}

func (c *MCPCmd) Run(ctx *RunContext) error {
	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if c.HTTP == "" {
		srv, _, err := buildMCPServer(c.Profile)
		if err != nil {
			return err
		}
		return srv.Run(runCtx, &mcp.StdioTransport{})
	}
	return c.runHTTP(runCtx)
}

func (c *MCPCmd) runHTTP(ctx context.Context) error {
	legacyToken, err := c.resolveToken()
	if err != nil {
		return err
	}
	safeKey, fullKey, err := resolveKeys()
	if err != nil {
		return err
	}

	addr, loopback := normalizeAddr(c.HTTP)

	handler, warnings, err := buildAuthHandler(c.Profile, safeKey, fullKey, legacyToken)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}

	// Key-routing mode is always authenticated (at least one key is set). The
	// no-token loopback refusal only applies to legacy single-token mode.
	if safeKey == "" && fullKey == "" {
		if legacyToken == "" && !loopback {
			return fmt.Errorf("refusing to serve MCP over non-loopback address %q without a token; set OLK_MCP_TOKEN", addr)
		}
		if legacyToken == "" {
			fmt.Fprintf(os.Stderr, "warning: serving MCP over HTTP on %s without a token; any local process can use this mailbox\n", addr)
		}
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally 0: streamable HTTP responses are long-lived;
		// per-request timeouts are enforced inside each tool handler instead.
	}

	errCh := make(chan error, 1)
	go func() {
		serveErr := httpSrv.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	fmt.Fprintf(os.Stderr, "olk MCP server listening on http://%s (%s)\n", addr, describeReach(c.Profile, safeKey, fullKey))

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}

// describeReach summarizes which profiles a running HTTP server exposes, for the
// startup log line.
func describeReach(profile, safeKey, fullKey string) string {
	if safeKey == "" && fullKey == "" {
		return profile + " profile"
	}
	var reach []string
	if safeKey != "" {
		reach = append(reach, "safe")
	}
	if fullKey != "" {
		reach = append(reach, "full")
	}
	return "key-routing: " + strings.Join(reach, "+") + " reachable"
}

// buildAuthHandler selects the HTTP auth mode and returns the wrapped handler
// plus any operator warnings. Key-routing mode is active when at least one
// per-profile key is set; otherwise the legacy single-token mode is used. It is
// socket-free so the mode-selection logic can be unit-tested.
func buildAuthHandler(profile, safeKey, fullKey, legacyToken string) (http.Handler, []string, error) {
	if safeKey == "" && fullKey == "" {
		srv, _, err := buildMCPServer(profile)
		if err != nil {
			return nil, nil, err
		}
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
		var warnings []string
		if profile == "full" {
			warnings = append(warnings, "the 'full' profile exposes destructive tools (delete, rm) over HTTP")
		}
		return securityMiddleware(legacyToken, handler), warnings, nil
	}

	// Identical keys would silently grant full access via the full-first check
	// in keyRouter, so refuse rather than pick a winner.
	if safeKey != "" && fullKey != "" && secureCompare(safeKey, fullKey) {
		return nil, nil, errors.New("OLK_MCP_KEY_SAFE and OLK_MCP_KEY_FULL must differ; identical keys would silently grant the full profile")
	}

	var warnings []string
	var safeH, fullH http.Handler
	if safeKey != "" {
		srv, _, err := buildMCPServer("safe")
		if err != nil {
			return nil, nil, err
		}
		safeH = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	}
	if fullKey != "" {
		srv, _, err := buildMCPServer("full")
		if err != nil {
			return nil, nil, err
		}
		fullH = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
		warnings = append(warnings, "the 'full' profile is reachable over HTTP and exposes destructive tools (delete, rm)")
	}
	if legacyToken != "" {
		warnings = append(warnings, "OLK_MCP_TOKEN/--http-token is ignored while per-profile keys (OLK_MCP_KEY_*) are set")
	}
	if profile == "full" {
		warnings = append(warnings, "--profile/OLK_MCP_PROFILE is ignored while per-profile keys are set; the presented key selects the profile")
	}
	return keyRouter(safeKey, fullKey, safeH, fullH), warnings, nil
}

// resolveToken returns the HTTP bearer token. Precedence: --http-token /
// OLK_MCP_TOKEN (both map to HTTPToken) > OLK_MCP_TOKEN_FILE. Passing the token
// as an inline flag is warned against.
func (c *MCPCmd) resolveToken() (string, error) {
	if c.HTTPToken != "" {
		for _, a := range os.Args {
			if a == "--http-token" || strings.HasPrefix(a, "--http-token=") {
				fmt.Fprintln(os.Stderr, "warning: --http-token may be visible in process listings and shell history; prefer the OLK_MCP_TOKEN env var")
				break
			}
		}
		return c.HTTPToken, nil
	}
	if f := os.Getenv("OLK_MCP_TOKEN_FILE"); f != "" {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("reading OLK_MCP_TOKEN_FILE: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

// resolveKeys returns the per-profile HTTP API keys. Each is read from its env
// var, else its _FILE variant. Keys are env-only (no CLI flag) so they cannot
// leak via process listings or shell history.
func resolveKeys() (safe, full string, err error) {
	if safe, err = resolveKey("OLK_MCP_KEY_SAFE"); err != nil {
		return "", "", err
	}
	if full, err = resolveKey("OLK_MCP_KEY_FULL"); err != nil {
		return "", "", err
	}
	return safe, full, nil
}

// resolveKey resolves a single key: the env value if set, else the trimmed
// contents of the file named by <env>_FILE. A missing/unreadable file is an
// error; a file that trims to empty resolves to "" (treated as not configured)
// with a warning, since an empty secret file is more likely a misconfiguration
// than an intentional fallback to an unauthenticated mode.
func resolveKey(env string) (string, error) {
	if v := os.Getenv(env); v != "" {
		return v, nil
	}
	fileEnv := env + "_FILE"
	f := os.Getenv(fileEnv)
	if f == "" {
		return "", nil
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", fileEnv, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		fmt.Fprintf(os.Stderr, "warning: %s is set but the file is empty; this key is not configured\n", fileEnv)
	}
	return key, nil
}

// parseBearer extracts the token from an "Authorization: Bearer <token>" header,
// or "" if the header is absent or malformed. Shared by both auth paths so their
// parsing semantics stay identical.
func parseBearer(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func securityMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !originAllowed(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if token != "" {
			tok := parseBearer(r)
			if tok == "" || !secureCompare(tok, token) {
				writeUnauthorized(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// keyRouter dispatches each request to the safe or full handler based on the
// presented bearer key. The full key is checked first; equal keys are rejected
// at startup so that ordering cannot silently escalate a safe key. A profile
// with no configured key (nil handler) is unreachable.
func keyRouter(safeKey, fullKey string, safeH, fullH http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !originAllowed(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		tok := parseBearer(r)
		if tok == "" {
			writeUnauthorized(w)
			return
		}
		switch {
		case fullKey != "" && secureCompare(tok, fullKey):
			fullH.ServeHTTP(w, r)
		case safeKey != "" && secureCompare(tok, safeKey):
			safeH.ServeHTTP(w, r)
		default:
			writeUnauthorized(w)
		}
	})
}

// secureCompare compares two secrets in constant time without leaking length.
func secureCompare(a, b string) bool {
	ah := sha256.Sum256([]byte(a))
	bh := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ah[:], bh[:]) == 1
}

// originAllowed mitigates DNS-rebinding: browser requests carry an Origin, which
// must be loopback. Non-browser MCP clients send no Origin and are allowed.
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// normalizeAddr resolves a listen address and reports whether it binds loopback
// only. A port-only value ("8765" or ":8765") is bound to 127.0.0.1 so it is
// not inadvertently exposed on all interfaces.
func normalizeAddr(addr string) (string, bool) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if !strings.Contains(addr, ":") {
			return "127.0.0.1:" + addr, true
		}
		return addr, false
	}
	if host == "" {
		return "127.0.0.1:" + port, true
	}
	return addr, isLoopbackHost(host)
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
