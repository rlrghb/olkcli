//go:build !windows

package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadLocalFile(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readLocalFile(small, 5); err != nil || string(data) != "hello" {
		t.Errorf("small file = %q, %v", data, err)
	}

	// A sparse 1 GiB file is refused from its size, without being read.
	huge := filepath.Join(dir, "huge.bin")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"over the limit": small, // limit 4 below
		"huge":           huge,
		"directory":      dir,
		"device":         "/dev/zero",
		"pipe":           fifo,
	} {
		limit := int64(35 << 20)
		if name == "over the limit" {
			limit = 4
		}
		done := make(chan error, 1)
		go func() { _, err := readLocalFile(path, limit); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				t.Errorf("%s: accepted", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: still reading after 5s", name)
		}
	}
}

// mail send refuses an endless attachment before signing in or reading it.
func TestMailSendRefusesADeviceAttachment(t *testing.T) {
	_, calls, err := runMailCommand(t, []string{"mail", "send"},
		[]string{"--to", "a@example.com", "--subject", "s", "--body", "b", "--attach", "/dev/zero"},
		func(req *http.Request) *http.Response { return graphJSONResponse(req, `{}`) })
	if err == nil || !strings.Contains(err.Error(), "not a regular file") || calls != 0 {
		t.Errorf("err = %v, calls = %d; want a not-regular-file refusal and no request", err, calls)
	}
}

// A pipe swapped in after the path check is opened without waiting for a
// writer, so the open file's own check can refuse it.
func TestOpenNoBlockDoesNotWaitOnAPipe(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := openNoBlock(fifo)
		if err == nil {
			_ = f.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("openNoBlock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("opening a pipe blocked")
	}
}
