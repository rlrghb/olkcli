package cmd

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rlrghb/olkcli/internal/msauth"
)

func TestLoginEarlierBudgetGetsExplanation(t *testing.T) {
	loginCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Model time spent obtaining the code before its equally long timer starts.
	time.Sleep(100 * time.Millisecond)
	_, err := msauth.PollForToken(loginCtx, "00000000-0000-0000-0000-000000000001", "common", "fictional-code", 5, 2, "", false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("library error = %v, want caller deadline", err)
	}
	got := explainLoginTimeout(loginCtx, fmt.Errorf("polling for token: %w", err), false)
	if !errors.Is(got, errLoginTimedOut) {
		t.Fatalf("command error = %v, want login-budget explanation", got)
	}
}

func TestLoginBrowserTimeoutKeepsRecoveryAdvice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("empty PATH does not disable the Windows system browser launcher")
	}
	// Prevent open/xdg-open from launching a browser. With no callback, the
	// real browser flow times out locally before any token or profile request.
	t.Setenv("PATH", t.TempDir())
	auth := msauth.NewAuthenticator(nil, "00000000-0000-0000-0000-000000000001", "common")
	loginCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := auth.LoginAuthCode(loginCtx, []string{"User.Read"}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("browser flow error = %v, want deadline", err)
	}
	got := explainLoginTimeout(loginCtx, err, true)
	for _, hint := range []string{"AADSTS50011", "http://localhost/callback", "without --browser"} {
		if !strings.Contains(err.Error(), hint) {
			t.Fatalf("browser flow no longer supplies expected advice %q", hint)
		}
		if got == nil || !strings.Contains(got.Error(), hint) {
			t.Errorf("command error %q lost browser recovery advice %q", got, hint)
		}
	}
}
