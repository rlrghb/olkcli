package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// captureMu serializes every tool invocation. captureStd reassigns the
// process-global os.Stdout/os.Stderr, so two captures must never run
// concurrently. This makes MCP tool execution effectively single-flight; the
// limitation is documented in the mcp command help.
var captureMu sync.Mutex

// captureStd runs fn while redirecting os.Stdout and os.Stderr to in-memory
// pipes, returning everything fn wrote to each. The original streams are always
// restored, even if fn panics (the panic is converted into an error). This is
// what keeps command output (221 direct os.Stdout writes plus the outfmt
// Printer) from corrupting the MCP transport's own use of stdout.
func captureStd(fn func() error) (stdoutText, stderrText string, runErr error) {
	captureMu.Lock()
	defer captureMu.Unlock()

	origOut, origErr := os.Stdout, os.Stderr

	outR, outW, err := os.Pipe()
	if err != nil {
		return "", "", fmt.Errorf("creating stdout pipe: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outW.Close()
		outR.Close()
		return "", "", fmt.Errorf("creating stderr pipe: %w", err)
	}

	os.Stdout, os.Stderr = outW, errW

	var (
		wg     sync.WaitGroup
		outBuf []byte
		errBuf []byte
	)
	wg.Add(2)
	go func() { defer wg.Done(); outBuf, _ = io.ReadAll(outR) }()
	go func() { defer wg.Done(); errBuf, _ = io.ReadAll(errR) }()

	func() {
		defer func() {
			if r := recover(); r != nil {
				runErr = fmt.Errorf("panic: %v", r)
			}
		}()
		runErr = fn()
	}()

	// Restore globals first, then close the write ends so the drain goroutines
	// observe EOF.
	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	wg.Wait()
	outR.Close()
	errR.Close()

	return string(outBuf), string(errBuf), runErr
}
