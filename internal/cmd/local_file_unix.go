//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// openNoBlock opens path for reading without waiting for a writer, as opening
// a FIFO otherwise would. Reads from a regular file are unaffected.
func openNoBlock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
