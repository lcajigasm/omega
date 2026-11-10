package cli

import (
	"fmt"
	"strings"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/llm"
	"github.com/lcajigasm/omega/internal/secrets"
)

// ProviderStatus is one row of the setup screen: what `omega setup` lists.
type ProviderStatus struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	KeyURL  string `json:"keyURL"`
	Local   bool   `json:"local"`   // runs on this machine: no key
	HasKey  bool   `json:"hasKey"`  // a key is stored or in the environment
	Status  string `json:"status"`  // "key in Keychain", "running on this Mac"…
	Current bool   `json:"current"` // the configured provider
	// AsksURL is set for the provider that is only an endpoint; BaseURL is
	// the one configured, if any.
	AsksURL bool   `json:"asksURL"`
	BaseURL string `json:"baseURL"`
}

// SetupProviders lists the providers with what is known about each, for a
// frontend that draws its own setup screen.
func SetupProviders() ([]ProviderStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var out []ProviderStatus
	for _, p := range llm.Providers {
		st := ProviderStatus{Name: p.Name, Label: p.Label, KeyURL: p.KeyURL, Local: p.Local,
			Current: p.Name == cfg.Provider, AsksURL: p.Name == "custom", BaseURL: cfg.BaseURLFor(p.Name)}
		if p.Local {
			base := st.BaseURL
			if base == "" {
				base = p.BaseURL
			}
			if base != "" && probeLocal(base) {
				st.Status = i18n.T("setup.local_found")
			}
		} else if _, src, err := getKey(p.Name); err == nil {
			st.HasKey = true
			st.Status = i18n.T("setup.key_in", secrets.Describe(src))
		} else {
			st.Status = i18n.T("setup.no_key")
		}
		out = append(out, st)
	}
	return out, nil
}

// SetupResult is what SaveSetup did.
type SetupResult struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Notes    []string `json:"notes"` // warnings and where the key went
}

// saveKey stores a key. Tests replace it.
var saveKey = secrets.Set

// SaveSetup makes provider the one omega uses, as `omega setup` does. A key
// is checked with the provider before it is stored; an empty key keeps the
// one already there. baseURL is for the endpoint-only provider. The model
// stays automatic.
func SaveSetup(provider, key, baseURL string) (SetupResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return SetupResult{}, err
	}
	if err := cfg.Validate(); err != nil {
		return SetupResult{}, err
	}
	info, ok := llm.Lookup(provider)
	if !ok {
		return SetupResult{}, fmt.Errorf("%s", i18n.T("gui.unknown_prov", provider))
	}
	var notes strings.Builder
	key, baseURL = cleanKey(key), strings.TrimSpace(baseURL)
	switch {
	case !info.Local && key != "":
		if err := checkKeyTo(&notes, cfg, provider, key); err != nil {
			return SetupResult{}, err
		}
		where, err := saveKey(provider, key)
		if err != nil {
			return SetupResult{}, err
		}
		fmt.Fprintln(&notes, i18n.T("setup.key_saved", secrets.Describe(where)))
		if where == secrets.FromFile {
			fmt.Fprintln(&notes, i18n.T("secrets.no_keyring"))
		}
	case !info.Local:
		if _, _, err := getKey(provider); err != nil {
			return SetupResult{}, fmt.Errorf("%s", i18n.T("setup.no_key_entered"))
		}
	case provider == "custom":
		if baseURL == "" && cfg.BaseURLFor(provider) == "" {
			return SetupResult{}, fmt.Errorf("%s", i18n.T("gui.base_url_req"))
		}
		if baseURL != "" {
			pc := cfg.Providers[provider]
			pc.BaseURL = baseURL
			cfg.Providers[provider] = pc
		}
	}

	cfg.Provider = provider
	cfg.Model = cfg.Providers[provider].Model
	if err := cfg.Save(); err != nil {
		return SetupResult{}, err
	}
	res := SetupResult{Provider: provider}
	for _, line := range strings.Split(strings.TrimSpace(notes.String()), "\n") {
		if line != "" {
			res.Notes = append(res.Notes, line)
		}
	}
	// The catalogue decides the automatic models; without it (offline, a
	// local server that is not running) the provider is saved all the same.
	if p, err := newProvider(cfg, info, ""); err == nil {
		loadModels(cfg, info, p, false)
	}
	if plan, err := planModel(cfg, info, "", "full"); err == nil {
		res.Model = plan.Model
	}
	return res, nil
}

// CurrentProvider is the configured provider's label and whether it can be
// used as it is: a local one, or one with a key.
func CurrentProvider() (label string, ready bool) {
	cfg, err := config.Load()
	if err != nil {
		return "", false
	}
	info, ok := llm.Lookup(cfg.Provider)
	if !ok {
		return cfg.Provider, false
	}
	if info.Local {
		return info.Label, true
	}
	_, _, err = getKey(info.Name)
	return info.Label, err == nil
}
