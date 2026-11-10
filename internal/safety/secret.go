package safety

import (
	"path"
	"strings"
)

// SecretFile reports a path whose contents are a private key or a credential
// store. Those bytes must not be sent to the model. A public key, known_hosts
// and an SSH config are not secret. The directory itself is not either, so
// listing ~/.ssh stays allowed.
func SecretFile(raw string) bool {
	p := normalizeSecretPath(raw)
	if p == "" || p == "." || strings.HasPrefix(path.Base(p), "-") {
		return false
	}
	base := strings.ToLower(path.Base(p))
	if strings.HasSuffix(base, ".pub") || base == "known_hosts" || base == "authorized_keys" || base == "environment" {
		return false
	}
	lower := strings.ToLower(p)
	if strings.Contains(lower, "/.ssh/") {
		return base != "config"
	}
	if strings.HasSuffix(lower, "/.aws/credentials") || strings.HasSuffix(lower, "/.aws/config") {
		return true
	}
	if strings.HasSuffix(lower, "/.kube/config") || strings.HasSuffix(lower, "/.config/gh/hosts.yml") {
		return true
	}
	if strings.Contains(lower, "/keychains/") || strings.Contains(base, ".keychain") {
		return true
	}
	// omega's own key file (the Linux fallback) and the GNOME keyring stores.
	if strings.HasSuffix(lower, "/omega/keys") || strings.Contains(lower, "/.local/share/keyrings/") {
		return true
	}
	if base == "credentials" || base == "credentials.json" {
		return strings.Contains(lower, "/")
	}
	return secretBasename(base)
}

func secretBasename(base string) bool {
	switch base {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
		".netrc", "_netrc", ".npmrc", ".pypirc", ".env":
		return true
	}
	if strings.HasPrefix(base, ".env.") && base != ".env.example" && base != ".env.sample" && base != ".env.template" {
		return true
	}
	switch path.Ext(base) {
	case ".pem", ".p12", ".pfx", ".key", ".kdbx", ".keychain-db":
		return true
	}
	return false
}

func normalizeSecretPath(raw string) string {
	p := strings.TrimSpace(raw)
	if len(p) >= 2 {
		q := p[0]
		if (q == '\'' || q == '"') && p[len(p)-1] == q {
			p = p[1 : len(p)-1]
		}
	}
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.ReplaceAll(p, "${HOME}", homeSentinel)
	p = strings.ReplaceAll(p, "$HOME", homeSentinel)
	if strings.HasPrefix(p, "~") {
		rest := strings.TrimPrefix(p, "~")
		if rest == "" || strings.HasPrefix(rest, "/") {
			p = homeSentinel + rest
		}
	}
	return path.Clean(p)
}

// leaksSecret reports a command that would print a private key, a credential
// file, or a password kept in the Keychain or a Linux keyring. The whole command is blocked, including under
// --yes, because the output is sent to the model.
func leaksSecret(part string) bool {
	words := commandWords(part)
	if len(words) == 0 {
		return false
	}
	switch path.Base(commandVerb(words)) {
	case "security":
		if securityReveals(words) {
			return true
		}
	case "secret-tool":
		// lookup and search print the secrets a Linux keyring holds.
		for _, w := range words {
			if w == "lookup" || w == "search" {
				return true
			}
		}
	}
	for _, w := range words {
		if SecretFile(w) {
			return true
		}
	}
	return false
}

func commandVerb(words []string) string {
	i := 0
	for i < len(words) {
		w := words[i]
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "-") {
			i++
			continue
		}
		switch path.Base(w) {
		case "sudo", "command", "exec", "nohup", "time":
			i++
			for i < len(words) && strings.HasPrefix(words[i], "-") {
				i++
			}
			continue
		}
		return w
	}
	return ""
}

func securityReveals(words []string) bool {
	blob := strings.Join(words, " ")
	if strings.Contains(blob, "dump-keychain") || strings.Contains(blob, "dump-trust-settings") {
		return true
	}
	reveal := false
	for _, w := range words {
		if w == "-w" || w == "-g" || w == "--password" {
			reveal = true
		}
	}
	if !reveal {
		return false
	}
	return strings.Contains(blob, "find-generic-password") ||
		strings.Contains(blob, "find-internet-password") ||
		strings.Contains(blob, "find-certificate")
}
