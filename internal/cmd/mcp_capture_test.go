package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/rlrghb/olkcli/internal/outfmt"
)

func TestCaptureStd_DirectAndPrinter(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr

	stdout, stderr, err := captureStd(func() error {
		fmt.Println("direct stdout line")
		fmt.Fprintln(os.Stderr, "direct stderr line")
		// outfmt.Printer defaults to os.Stdout, which is redirected during capture.
		p := outfmt.NewPrinter(true, false, false, "", "UTC", false)
		return p.PrintJSON(map[string]string{"k": "v"}, 1, "")
	})
	if err != nil {
		t.Fatalf("captureStd returned err: %v", err)
	}

	if !strings.Contains(stdout, "direct stdout line") {
		t.Errorf("stdout missing direct write: %q", stdout)
	}
	if !strings.Contains(stdout, "\"k\"") {
		t.Errorf("stdout missing printer JSON: %q", stdout)
	}
	if !strings.Contains(stderr, "direct stderr line") {
		t.Errorf("stderr missing direct write: %q", stderr)
	}

	// Globals must be restored.
	if os.Stdout != origOut || os.Stderr != origErr {
		t.Error("os.Stdout/os.Stderr not restored after captureStd")
	}
}

func TestCaptureStd_RestoresOnPanic(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr

	_, _, err := captureStd(func() error {
		panic("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Errorf("expected panic error, got %v", err)
	}
	if os.Stdout != origOut || os.Stderr != origErr {
		t.Error("os.Stdout/os.Stderr not restored after panic")
	}
}

// TestCaptureStd_Concurrent runs under -race to confirm the mutex serializes
// access to the process-global streams without interleaving output.
func TestCaptureStd_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			want := fmt.Sprintf("line-%d", n)
			out, _, err := captureStd(func() error {
				fmt.Print(want)
				return nil
			})
			if err != nil {
				t.Errorf("captureStd: %v", err)
			}
			if out != want {
				t.Errorf("got %q, want %q (interleaved capture)", out, want)
			}
		}(i)
	}
	wg.Wait()
}
