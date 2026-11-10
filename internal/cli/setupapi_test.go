package cli

import (
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/secrets"
)

// isolate points the configuration at an empty directory and keeps keys out
// of the real keychain.
func isolate(t *testing.T, keys map[string]string) *[]string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var saved []string
	oldGet, oldSave := getKey, saveKey
	getKey = func(p string) (string, secrets.Source, error) {
		if k, ok := keys[p]; ok {
			return k, secrets.FromFile, nil
		}
		return "", "", secrets.ErrNotFound
	}
	saveKey = func(p, k string) (secrets.Source, error) {
		saved = append(saved, p+"="+k)
		return secrets.FromFile, nil
	}
	t.Cleanup(func() { getKey, saveKey = oldGet, oldSave })
	return &saved
}

func TestSaveSetupLocalProvider(t *testing.T) {
	saved := isolate(t, nil)
	res, err := SaveSetup("ollama", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "ollama" || len(*saved) != 0 {
		t.Fatalf("result %+v, saved %v", res, *saved)
	}
	cfg, _ := config.Load()
	if cfg.Provider != "ollama" {
		t.Errorf("provider in the config: %q", cfg.Provider)
	}
	if label, ready := CurrentProvider(); !ready || !strings.Contains(label, "Ollama") {
		t.Errorf("current: %q %v", label, ready)
	}
}

func TestSaveSetupRefusesWhatCannotWork(t *testing.T) {
	saved := isolate(t, nil)
	for _, c := range []struct{ provider, key, url string }{
		{"anthropic", "", ""},                      // no key given and none stored
		{"anthropic", "https://example.com/k", ""}, // a URL is not a key
		{"anthropic", "short", ""},
		{"custom", "", ""}, // an endpoint with no URL
		{"nope", "", ""},
	} {
		if _, err := SaveSetup(c.provider, c.key, c.url); err == nil {
			t.Errorf("%v was accepted", c)
		}
	}
	if len(*saved) != 0 {
		t.Errorf("a key was stored: %v", *saved)
	}
	// Nothing was written: the provider is still the default.
	if cfg, _ := config.Load(); cfg.Provider != "anthropic" {
		t.Errorf("provider changed to %q", cfg.Provider)
	}
	if _, ready := CurrentProvider(); ready {
		t.Error("a provider with no key is reported ready")
	}
}

func TestSaveSetupKeepsStoredKeyAndCustomURL(t *testing.T) {
	saved := isolate(t, map[string]string{"openai": "sk-stored-key-0123456789"})
	if _, err := SaveSetup("openai", "  ", ""); err != nil {
		t.Fatalf("an empty key must keep the stored one: %v", err)
	}
	if len(*saved) != 0 {
		t.Errorf("the stored key was rewritten: %v", *saved)
	}
	if _, err := SaveSetup("custom", "", " http://localhost:8080/v1 "); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load()
	if cfg.Provider != "custom" || cfg.BaseURLFor("custom") != "http://localhost:8080/v1" {
		t.Errorf("config: %q %q", cfg.Provider, cfg.BaseURLFor("custom"))
	}
}

func TestSetupProviders(t *testing.T) {
	isolate(t, map[string]string{"openrouter": "sk-or-stored-0123456789"})
	list, err := SetupProviders()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ProviderStatus{}
	for _, p := range list {
		by[p.Name] = p
	}
	if !by["openrouter"].HasKey || by["anthropic"].HasKey {
		t.Errorf("keys: %+v %+v", by["openrouter"], by["anthropic"])
	}
	if !by["anthropic"].Current || !by["ollama"].Local || !by["custom"].AsksURL {
		t.Errorf("flags wrong: %+v", list)
	}
}
