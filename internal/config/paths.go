package config

import (
	"os"
	"path/filepath"
)

// DefaultNamespace is the storage namespace of released and installed builds.
const DefaultNamespace = "olk"

// Namespace names the configuration directory and the credential-store
// entries this binary uses. Released builds keep DefaultNamespace. `make
// build` sets it at link time to "olk-dev", so a development binary keeps its
// own accounts and tokens and never reads or rewrites an installed binary's
// keychain items, which on macOS would otherwise raise Keychain prompts for
// whichever of the two builds did not create the item.
var Namespace = DefaultNamespace

func ConfigDir() string {
	if dir := os.Getenv("OLK_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserConfigDir()
	if err != nil {
		home = os.Getenv("HOME")
		return filepath.Join(home, ".config", Namespace)
	}
	return filepath.Join(home, Namespace)
}

func AccountsDir() string {
	return filepath.Join(ConfigDir(), "accounts")
}

func ConfigFilePath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func EnsureConfigDir() error {
	dirs := []string{ConfigDir(), AccountsDir()}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		// Enforce restrictive permissions even if directory already existed
		// with weaker permissions (e.g. created by another tool or umask).
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
