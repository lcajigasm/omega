// Package secrets stores API keys where the operating system keeps secrets:
// the macOS Keychain, or on Linux the Secret Service (through secret-tool)
// with a file only the user can read as the fallback. Environment variables
// take precedence everywhere so CI or one-off runs can override them.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
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
var ErrNotFound = errors.New("no API key configured")

// Source describes where a key came from.
type Source string

const (
	FromEnv           Source = "env"
	FromKeychain      Source = "keychain"
	FromSecretService Source = "secret-service"
	FromFile          Source = "file"
)

// Describe is the source in words, for messages to the user.
func Describe(s Source) string {
	switch s {
	case FromEnv:
		return i18n.T("secrets.src.env")
	case FromKeychain:
		return i18n.T("secrets.src.keychain")
	case FromSecretService:
		return i18n.T("secrets.src.secret_service")
	case FromFile:
		return i18n.T("secrets.src.file", FilePath())
	}
	return string(s)
}

// Seams the tests replace.
var (
	goos     = runtime.GOOS
	lookPath = exec.LookPath
	// runTool runs a program with stdin and returns its combined output.
	runTool = func(timeout time.Duration, stdin string, name string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
)

// FilePath is the fallback key file on Linux (~/.config/omega/keys).
func FilePath() string { return filepath.Join(config.Dir(), "keys") }

// Get returns the API key for a provider and where it was found.
func Get(provider string) (string, Source, error) {
	if v := os.Getenv(EnvVar[provider]); v != "" {
		return v, FromEnv, nil
	}
	switch goos {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-s", service, "-a", provider, "-w").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), FromKeychain, nil
		}
	case "linux":
		if key := serviceGet(provider); key != "" {
			return key, FromSecretService, nil
		}
		if key := readFile()[provider]; key != "" {
			return key, FromFile, nil
		}
	}
	return "", "", &notFoundError{provider}
}

// Set stores (or replaces) the key and reports where it went: the login
// Keychain on macOS; on Linux the Secret Service when secret-tool can reach
// one, otherwise the key file.
func Set(provider, key string) (Source, error) {
	switch goos {
	case "darwin":
		cmd := exec.Command("security", "add-generic-password", "-U", "-s", service, "-a", provider, "-w", key,
			"-l", "omega ("+provider+")")
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s", i18n.T("secrets.save", err, strings.TrimSpace(string(out))))
		}
		return FromKeychain, nil
	case "linux":
		if serviceSet(provider, key) {
			// A copy left in the file would be stale from now on.
			keys := readFile()
			if _, ok := keys[provider]; ok {
				delete(keys, provider)
				if err := writeFile(keys); err != nil {
					return "", err
				}
			}
			return FromSecretService, nil
		}
		keys := readFile()
		keys[provider] = key
		if err := writeFile(keys); err != nil {
			return "", err
		}
		return FromFile, nil
	}
	return "", fmt.Errorf("%s", i18n.T("secrets.unsupported", goos, EnvVar[provider]))
}

// Delete removes the key from wherever Set put it.
func Delete(provider string) error {
	switch goos {
	case "darwin":
		if out, err := exec.Command("security", "delete-generic-password", "-s", service, "-a", provider).CombinedOutput(); err != nil {
			return fmt.Errorf("%s", i18n.T("secrets.delete", err, strings.TrimSpace(string(out))))
		}
		return nil
	case "linux":
		found := false
		if serviceGet(provider) != "" {
			found = true
			if out, err := runTool(30*time.Second, "", "secret-tool", "clear", "service", service, "account", provider); err != nil {
				return fmt.Errorf("%s", i18n.T("secrets.delete_service", err, strings.TrimSpace(out)))
			}
		}
		keys := readFile()
		if _, ok := keys[provider]; ok {
			found = true
			delete(keys, provider)
			if err := writeFile(keys); err != nil {
				return err
			}
		}
		if !found {
			return fmt.Errorf("%s", i18n.T("secrets.none", provider))
		}
		return nil
	}
	return fmt.Errorf("%s", i18n.T("secrets.unsupported", goos, EnvVar[provider]))
}

// Store is where Set would put a key on this machine right now, or "" on a
// system omega has no key storage for.
func Store() Source {
	switch goos {
	case "darwin":
		return FromKeychain
	case "linux":
		if serviceUp() {
			return FromSecretService
		}
		return FromFile
	}
	return ""
}

// ---- Secret Service (Linux), through secret-tool from libsecret ----

// serviceUp reports whether a Secret Service answers: a lookup of a name
// that does not exist fails silently when one does, and with an error
// message (no session bus, no keyring daemon) when none does.
func serviceUp() bool {
	if !hasSecretTool() {
		return false
	}
	out, err := runTool(15*time.Second, "", "secret-tool", "lookup", "service", service, "account", "omega-probe")
	return err == nil || strings.TrimSpace(out) == ""
}

func hasSecretTool() bool {
	_, err := lookPath("secret-tool")
	return err == nil
}

// serviceGet returns "" when secret-tool is missing, no Secret Service
// answers (no session bus, no keyring daemon) or nothing is stored.
func serviceGet(provider string) string {
	if !hasSecretTool() {
		return ""
	}
	out, err := runTool(30*time.Second, "", "secret-tool", "lookup", "service", service, "account", provider)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// serviceSet reports whether the key was stored in the Secret Service.
func serviceSet(provider, key string) bool {
	if !hasSecretTool() {
		return false
	}
	_, err := runTool(60*time.Second, key, "secret-tool", "store", "--label=omega ("+provider+")", "service", service, "account", provider)
	return err == nil
}

// ---- key file (Linux fallback): "provider=key" lines, mode 0600 ----

func readFile() map[string]string {
	keys := map[string]string{}
	path := FilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return keys
	}
	// The file holds keys: if something widened its permissions, narrow them.
	if fi, err := os.Stat(path); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		os.Chmod(path, 0o600)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if name, key, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) != "" {
			keys[strings.TrimSpace(name)] = strings.TrimSpace(key)
		}
	}
	return keys
}

// writeFile replaces the key file atomically; with no keys left it is removed.
func writeFile(keys map[string]string) error {
	path := FilePath()
	if len(keys) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%s", i18n.T("secrets.file", path, err))
		}
		return nil
	}
	fail := func(err error) error { return fmt.Errorf("%s", i18n.T("secrets.file", path, err)) }
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fail(err)
	}
	names := make([]string, 0, len(keys))
	for n := range keys {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# omega API keys. Only your user can read this file; do not share it.\n")
	for _, n := range names {
		fmt.Fprintf(&b, "%s=%s\n", n, keys[n])
	}
	// CreateTemp makes the file 0600 from the start: the key never sits in
	// a file other users can read.
	f, err := os.CreateTemp(filepath.Dir(path), ".keys-*")
	if err != nil {
		return fail(err)
	}
	tmp := f.Name()
	_, werr := f.WriteString(b.String())
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp, 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		os.Remove(tmp)
		return fail(werr)
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

type notFoundError struct{ provider string }

func (e *notFoundError) Error() string {
	return i18n.T("err.no_key", e.provider, e.provider, EnvVar[e.provider])
}

func (e *notFoundError) Unwrap() error { return ErrNotFound }
