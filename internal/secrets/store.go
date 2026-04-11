package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/99designs/keyring"

	"github.com/rlrghb/olkcli/internal/config"
)

const (
	serviceName = "olk"
	tokenPrefix = "olk:token:"
)

// Store defines the interface for credential storage.
type Store interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
	Keys() ([]string, error)
}

// KeyringStore implements Store using the keyring library for
// cross-platform credential storage (macOS Keychain, Linux Secret Service,
// Windows WinCred).
type KeyringStore struct {
	ring keyring.Keyring
}

// NewKeyringStore creates a new KeyringStore backed by the OS credential manager.
func NewKeyringStore() (*KeyringStore, error) {
	// Pre-create keyring fallback directory with restrictive permissions
	// to prevent other users from reading tokens on multi-user systems.
	keyringDir := filepath.Join(config.ConfigDir(), "keyring")
	if err := os.MkdirAll(keyringDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating keyring directory: %w", err)
	}
	// Ensure restrictive permissions even if the directory already existed.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(keyringDir, 0o700); err != nil {
			return nil, fmt.Errorf("setting keyring directory permissions: %w", err)
		}
	}

	cfg := keyring.Config{
		ServiceName: serviceName,

		// macOS
		KeychainTrustApplication:       true,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,

		// Linux / FreeBSD
		LibSecretCollectionName: serviceName,

		// Windows
		WinCredPrefix: serviceName,

		// Fall back to an encrypted file store when no native backend is
		// available (e.g. headless Linux without Secret Service).
		FileDir:          keyringDir,
		FilePasswordFunc: keyring.TerminalPrompt,
	}

	// On macOS, prefer Keychain (native UI with "Always Allow") but fall
	// back to file backend when cgo is unavailable (e.g. Homebrew binary).
	if runtime.GOOS == "darwin" {
		cfg.AllowedBackends = []keyring.BackendType{keyring.KeychainBackend, keyring.FileBackend}
	}

	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("opening keyring: %w", err)
	}
	return &KeyringStore{ring: ring}, nil
}

// Set stores a value under the given key.
func (s *KeyringStore) Set(key, value string) error {
	return s.ring.Set(keyring.Item{
		Key:  key,
		Data: []byte(value),
	})
}

// Get retrieves the value stored under the given key.
func (s *KeyringStore) Get(key string) (string, error) {
	item, err := s.ring.Get(key)
	if err != nil {
		return "", fmt.Errorf("retrieving stored credential: %w", err)
	}
	return string(item.Data), nil
}

// Delete removes the entry for the given key.
func (s *KeyringStore) Delete(key string) error {
	return s.ring.Remove(key)
}

// Keys returns all keys currently stored in the keyring.
func (s *KeyringStore) Keys() ([]string, error) {
	keys, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("listing keys: %w", err)
	}
	return keys, nil
}

// TokenKey returns the canonical keyring key for a given email address.
// Format: olk:token:<email>
func TokenKey(email string) string {
	return tokenPrefix + strings.ToLower(email)
}
