package cmd

import (
	"fmt"
	"io"
	"os"
)

// readLocalFile reads a regular file of at most limit bytes. The path is
// checked before opening, so a device or pipe is refused without blocking on
// it, and the open file is checked again, so one swapped in between is refused
// too. It never reads more than limit+1 bytes, so a file that grows after the
// check is refused rather than loaded.
func readLocalFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if err := checkLocalFile(path, info, limit); err != nil {
		return nil, err
	}
	// Opened without blocking, so a pipe swapped in since the check is
	// refused below rather than waited on.
	f, err := openNoBlock(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err = f.Stat(); err != nil {
		return nil, err
	}
	if err := checkLocalFile(path, info, limit); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s grew past %d bytes while being read", path, limit)
	}
	return data, nil
}

func checkLocalFile(path string, info os.FileInfo, limit int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > limit {
		return fmt.Errorf("%s is %d bytes, over the %d byte limit", path, info.Size(), limit)
	}
	return nil
}
