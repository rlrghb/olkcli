package secrets

import (
	"testing"

	"github.com/99designs/keyring"

	"github.com/rlrghb/olkcli/internal/config"
)

func TestItemLabelNamesTheAccountForTokens(t *testing.T) {
	tests := []struct{ key, want string }{
		{TokenKey("Someone@Example.com"), "olk token for someone@example.com"},
		{"olk:other", "olk olk:other"},
	}
	for _, tc := range tests {
		if got := ItemLabel(tc.key); got != tc.want {
			t.Fatalf("ItemLabel(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func useNamespace(t *testing.T, ns string) {
	t.Helper()
	orig := config.Namespace
	config.Namespace = ns
	t.Cleanup(func() { config.Namespace = orig })
}

func TestItemLabelNamesTheBuildNamespace(t *testing.T) {
	useNamespace(t, "olk-dev")
	tests := []struct{ key, want string }{
		{TokenKey("someone@example.com"), "olk-dev token for someone@example.com"},
		{"olk:other", "olk-dev olk:other"},
	}
	for _, tc := range tests {
		if got := ItemLabel(tc.key); got != tc.want {
			t.Fatalf("ItemLabel(%q) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestKeyringConfigKeepsReleaseStorageNames(t *testing.T) {
	cfg := keyringConfig(t.TempDir(), nil)
	if cfg.ServiceName != "olk" || cfg.LibSecretCollectionName != "olk" || cfg.WinCredPrefix != "olk" {
		t.Fatalf("release config = service %q, collection %q, wincred %q; want olk for all",
			cfg.ServiceName, cfg.LibSecretCollectionName, cfg.WinCredPrefix)
	}
	if cfg.PassPrefix != "" {
		t.Fatalf("release PassPrefix = %q, want empty so existing pass entries are found", cfg.PassPrefix)
	}
}

func TestKeyringConfigSeparatesADevelopmentNamespace(t *testing.T) {
	useNamespace(t, "olk-dev")
	cfg := keyringConfig(t.TempDir(), nil)
	for name, got := range map[string]string{
		"ServiceName":             cfg.ServiceName,
		"LibSecretCollectionName": cfg.LibSecretCollectionName,
		"WinCredPrefix":           cfg.WinCredPrefix,
		"PassPrefix":              cfg.PassPrefix,
	} {
		if got != "olk-dev" {
			t.Errorf("%s = %q, want olk-dev", name, got)
		}
	}
}

func newFileStore(t *testing.T) *KeyringStore {
	t.Helper()
	ring, err := keyring.Open(keyring.Config{
		ServiceName:      config.Namespace,
		AllowedBackends:  []keyring.BackendType{keyring.FileBackend},
		FileDir:          t.TempDir(),
		FilePasswordFunc: keyring.FixedStringPrompt("test"),
	})
	if err != nil {
		t.Fatalf("open file keyring: %v", err)
	}
	return &KeyringStore{ring: ring}
}

func TestSetStoresTheLabelAndUpdatesInPlace(t *testing.T) {
	store := newFileStore(t)
	key := TokenKey("someone@example.com")
	if err := store.ring.Set(keyring.Item{Key: key, Data: []byte("old"), Label: ""}); err != nil {
		t.Fatalf("seed unlabelled item: %v", err)
	}

	if err := store.Set(key, "new"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	item, err := store.ring.Get(key)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if item.Label != ItemLabel(key) || string(item.Data) != "new" {
		t.Fatalf("item = label %q data %q, want the current label and new data", item.Label, item.Data)
	}
	if got, err := store.Get(key); err != nil || got != "new" {
		t.Fatalf("store.Get = %q, %v", got, err)
	}
}
