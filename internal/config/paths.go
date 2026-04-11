package config

import (
	"os"
	"path/filepath"
)

const appName = "olk"

func ConfigDir() string {
	if dir := os.Getenv("OLK_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserConfigDir()
	if err != nil {
		home = os.Getenv("HOME")
		return filepath.Join(home, ".config", appName)
	}
	return filepath.Join(home, appName)
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
	}
	return nil
}
