package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// useTempConfigDir points OLK_CONFIG_DIR at a t.TempDir and restores the prior
// env when the test ends.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("OLK_CONFIG_DIR", dir)
	return dir
}

func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	useTempConfigDir(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if cfg.DefaultAccount != "" {
		t.Errorf("DefaultAccount should be empty, got %q", cfg.DefaultAccount)
	}
	if cfg.Clients == nil {
		t.Errorf("Clients map should be initialized, got nil")
	}
	if len(cfg.Clients) != 0 {
		t.Errorf("Clients map should be empty, got %d entries", len(cfg.Clients))
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	useTempConfigDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.SetDefaultAccount("alice@example.com")
	cfg.SetClient("alice@example.com", Client{ClientID: "cid-1", TenantID: "tid-1"})
	cfg.SetClient("bob@example.com", Client{ClientID: "cid-2"})
	cfg.SetTimezone("America/Los_Angeles")

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.GetDefaultAccount() != "alice@example.com" {
		t.Errorf("default account: got %q", loaded.GetDefaultAccount())
	}
	if loaded.GetTimezone() != "America/Los_Angeles" {
		t.Errorf("timezone: got %q", loaded.GetTimezone())
	}
	a := loaded.GetClient("alice@example.com")
	if a.ClientID != "cid-1" || a.TenantID != "tid-1" {
		t.Errorf("alice client roundtrip: got %+v", a)
	}
	b := loaded.GetClient("bob@example.com")
	if b.ClientID != "cid-2" {
		t.Errorf("bob client roundtrip: got %+v", b)
	}
}

func TestGetClient_DefaultsForUnknownAccount(t *testing.T) {
	cfg := &Config{Clients: map[string]Client{}}
	got := cfg.GetClient("nobody@example.com")
	if got.ClientID != DefaultClientID {
		t.Errorf("default ClientID: got %q, want %q", got.ClientID, DefaultClientID)
	}
	if got.TenantID != DefaultTenantID {
		t.Errorf("default TenantID: got %q, want %q", got.TenantID, DefaultTenantID)
	}
}

func TestRemoveAccount(t *testing.T) {
	cfg := &Config{Clients: map[string]Client{}}
	cfg.SetClient("alice@example.com", Client{ClientID: "cid-1"})
	cfg.SetClient("bob@example.com", Client{ClientID: "cid-2"})
	cfg.SetDefaultAccount("alice@example.com")

	cfg.RemoveAccount("alice@example.com")

	if _, ok := cfg.Clients["alice@example.com"]; ok {
		t.Errorf("alice should be removed from Clients map")
	}
	if cfg.GetDefaultAccount() != "" {
		t.Errorf("DefaultAccount should be cleared when matching removed account; got %q", cfg.GetDefaultAccount())
	}
	if _, ok := cfg.Clients["bob@example.com"]; !ok {
		t.Errorf("unrelated account bob should still exist")
	}
}

func TestRemoveAccount_PreservesUnrelatedDefault(t *testing.T) {
	cfg := &Config{Clients: map[string]Client{}}
	cfg.SetClient("alice@example.com", Client{ClientID: "cid-1"})
	cfg.SetClient("bob@example.com", Client{ClientID: "cid-2"})
	cfg.SetDefaultAccount("bob@example.com")

	cfg.RemoveAccount("alice@example.com")

	if cfg.GetDefaultAccount() != "bob@example.com" {
		t.Errorf("DefaultAccount should be preserved when not matching removed account; got %q", cfg.GetDefaultAccount())
	}
}

func TestSave_PermissionsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only permission semantics")
	}
	useTempConfigDir(t)
	cfg := &Config{Clients: map[string]Client{}}
	cfg.SetDefaultAccount("alice@example.com")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(ConfigFilePath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config file perm: got %o, want 0600", got)
	}
}

func TestLoad_RejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink-creation requires elevated perms on windows")
	}
	dir := useTempConfigDir(t)

	// Create a real target file and symlink the config path to it.
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"default_account":"x"}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, ConfigFilePath()); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatalf("Load should refuse symlinked config file")
	}
}

func TestLoad_AutoFixesWorldReadablePerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only permission semantics")
	}
	useTempConfigDir(t)
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigFilePath(), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	info, err := os.Stat(ConfigFilePath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("Load should chmod to 0600; got %o", got)
	}
}

func TestLoad_RejectsMalformedJSON(t *testing.T) {
	useTempConfigDir(t)
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigFilePath(), []byte("{ not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatalf("Load should fail on malformed JSON")
	}
}

func TestConcurrentReadWrite_NoRace(t *testing.T) {
	// Run with -race in CI to validate the sync.RWMutex guarding the Config fields.
	cfg := &Config{Clients: map[string]Client{}}
	cfg.SetClient("alice@example.com", Client{ClientID: "cid-1"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cfg.SetClient("bob@example.com", Client{ClientID: "cid-2"})
			cfg.SetDefaultAccount("bob@example.com")
		}()
		go func() {
			defer wg.Done()
			_ = cfg.GetClient("alice@example.com")
			_ = cfg.GetDefaultAccount()
			_ = cfg.GetTimezone()
		}()
	}
	wg.Wait()
}

// TestSave_AtomicWritesProduceValidJSON sanity-checks that atomicWriteFile
// produces a complete, parseable file (no partial writes).
func TestSave_AtomicWritesProduceValidJSON(t *testing.T) {
	useTempConfigDir(t)
	cfg := &Config{Clients: map[string]Client{}}
	cfg.SetDefaultAccount("alice@example.com")
	cfg.SetClient("alice@example.com", Client{ClientID: "cid-1"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v\ncontent: %s", err, data)
	}
	if m["default_account"] != "alice@example.com" {
		t.Errorf("default_account: got %v", m["default_account"])
	}
}

func TestPaths_HonorsEnvOverride(t *testing.T) {
	t.Setenv("OLK_CONFIG_DIR", "/tmp/olk-test-xyzzy")
	if ConfigDir() != "/tmp/olk-test-xyzzy" {
		t.Errorf("ConfigDir should honor OLK_CONFIG_DIR; got %q", ConfigDir())
	}
	if ConfigFilePath() != filepath.Join(ConfigDir(), "config.json") {
		t.Errorf("ConfigFilePath: got %q", ConfigFilePath())
	}
	if AccountsDir() != filepath.Join(ConfigDir(), "accounts") {
		t.Errorf("AccountsDir: got %q", AccountsDir())
	}
}
