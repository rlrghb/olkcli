package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config represents the olk configuration
type Config struct {
	DefaultAccount string            `json:"default_account,omitempty"`
	Clients        map[string]Client `json:"clients,omitempty"`
	mu             sync.RWMutex
}

// Client represents an OAuth2 client configuration
type Client struct {
	ClientID string `json:"client_id"`
	TenantID string `json:"tenant_id,omitempty"`
}

const (
	DefaultClientID = "51e726d0-22a4-45f7-a71c-b472ff84c027"
	DefaultTenantID = "common"
)

// Load reads config from disk
func Load() (*Config, error) {
	cfg := &Config{
		Clients: make(map[string]Client),
	}

	path := ConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Clients == nil {
		cfg.Clients = make(map[string]Client)
	}
	return cfg, nil
}

// Save writes config to disk
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := EnsureConfigDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(ConfigFilePath(), data, 0o600)
}

// atomicWriteFile writes data to a temp file then renames it to the target path,
// preventing corruption from crashes or interrupts during write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// SetDefaultAccount sets the default account
func (c *Config) SetDefaultAccount(email string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DefaultAccount = email
}

// GetDefaultAccount returns the default account
func (c *Config) GetDefaultAccount() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DefaultAccount
}

// SetClient stores a client configuration for an account
func (c *Config) SetClient(email string, client Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Clients == nil {
		c.Clients = make(map[string]Client)
	}
	c.Clients[email] = client
}

// GetClient returns the client config for an account, or defaults
func (c *Config) GetClient(email string) Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cl, ok := c.Clients[email]; ok {
		return cl
	}
	return Client{
		ClientID: DefaultClientID,
		TenantID: DefaultTenantID,
	}
}

// RemoveAccount removes an account from config
func (c *Config) RemoveAccount(email string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Clients, email)
	if c.DefaultAccount == email {
		c.DefaultAccount = ""
	}
}
