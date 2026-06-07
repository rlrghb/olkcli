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
	srv, _, err := buildMCPServer(c.Profile)
	if err != nil {
		return err
	}

	// The MCP server is long-running, so it must not inherit the per-command
	// timeout that Execute applies to ctx.Ctx. Run until interrupted instead.
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if c.HTTP == "" {
		return srv.Run(runCtx, &mcp.StdioTransport{})
	}
	return c.runHTTP(runCtx, srv)
}

func (c *MCPCmd) runHTTP(ctx context.Context, srv *mcp.Server) error {
	token, err := c.resolveToken()
	if err != nil {
		return err
	}

	addr, loopback := normalizeAddr(c.HTTP)
	if token == "" && !loopback {
		return fmt.Errorf("refusing to serve MCP over non-loopback address %q without a token; set OLK_MCP_TOKEN", addr)
	}
	if token == "" {
		fmt.Fprintf(os.Stderr, "warning: serving MCP over HTTP on %s without a token; any local process can use this mailbox\n", addr)
	}
	if c.Profile == "full" {
		fmt.Fprintln(os.Stderr, "warning: the 'full' profile exposes destructive tools (delete, rm) over HTTP")
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	secured := securityMiddleware(token, handler)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           secured,
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

	fmt.Fprintf(os.Stderr, "olk MCP server (%s profile) listening on http://%s\n", c.Profile, addr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
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

func securityMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !originAllowed(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if token != "" {
			const prefix = "Bearer "
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, prefix) || !secureCompare(strings.TrimPrefix(h, prefix), token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
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
