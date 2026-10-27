// Package secrets stores API keys in the macOS Keychain, with environment
// variables taking precedence so CI or one-off runs can override them.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const service = "omega-cli"

// EnvVar is the environment variable checked for each provider's key.
var EnvVar = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"opencode":   "OPENCODE_API_KEY",
}

// ErrNotFound means no key is configured for the provider.
var ErrNotFound = errors.New("no hay API key configurada")

// Source describes where a key came from.
type Source string

const (
	FromEnv      Source = "env"
	FromKeychain Source = "keychain"
)

// Get returns the API key for a provider and where it was found.
func Get(provider string) (string, Source, error) {
	if v := os.Getenv(EnvVar[provider]); v != "" {
		return v, FromEnv, nil
	}
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-a", provider, "-w").Output()
	if err != nil {
		return "", "", fmt.Errorf("%w para %s (usa `omega auth set %s` o exporta %s)", ErrNotFound, provider, provider, EnvVar[provider])
	}
	return strings.TrimSpace(string(out)), FromKeychain, nil
}

// Set stores (or replaces) the key in the login Keychain.
func Set(provider, key string) error {
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", service, "-a", provider, "-w", key,
		"-l", "omega ("+provider+")")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("guardando en el llavero: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes the key from the Keychain.
func Delete(provider string) error {
	if out, err := exec.Command("security", "delete-generic-password", "-s", service, "-a", provider).CombinedOutput(); err != nil {
		return fmt.Errorf("borrando del llavero: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Mask shows only the edges of a key.
func Mask(key string) string {
	if len(key) <= 10 {
		return strings.Repeat("•", len(key))
	}
	return key[:6] + "…" + key[len(key)-4:]
}
