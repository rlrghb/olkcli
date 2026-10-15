//go:build windows

package cmd

import "os"

// openNoBlock opens path for reading. Windows has no FIFOs that block an open.
func openNoBlock(path string) (*os.File, error) { return os.Open(path) }
