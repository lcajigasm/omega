package safety

import (
	"os"
	"path"
	"regexp"
	"strings"
)

// Seams the tests replace.
var (
	getwd = os.Getwd
	// configHome is XDG_CONFIG_HOME as omega itself sees it.
	configHome = func() string { return os.Getenv("XDG_CONFIG_HOME") }
)

// xdgSentinel stands in for $XDG_CONFIG_HOME written in a command.
const xdgSentinel = "/xdg-config-home"

// omegaReadable are the files of omega's config folder a command may read,
// by exact name, as what follows the folder: the configuration, the notes
// and the omega mac rules. Everything else there is refused: the folder
// holds the API key file on Linux.
var omegaReadable = map[string]bool{"/config.toml": true, "/notes.md": true, "/mac.toml": true}

// underOmega reports a (cleaned, lower-case) path inside omega's config
// folder or the folder itself, and what follows the folder.
func underOmega(lower string) (rest string, ok bool) {
	if i := strings.Index(lower, "/.config/omega"); i >= 0 {
		rest = lower[i+len("/.config/omega"):]
		if rest == "" || strings.HasPrefix(rest, "/") {
			return rest, true
		}
	}
	dirs := []string{xdgSentinel + "/omega"}
	if x := configHome(); x != "" {
		dirs = append(dirs, strings.ToLower(path.Clean(strings.ReplaceAll(x, `\`, "/")))+"/omega")
	}
	for _, d := range dirs {
		if rest, found := strings.CutPrefix(lower, d); found && (rest == "" || strings.HasPrefix(rest, "/")) {
			return rest, true
		}
	}
	return "", false
}

// isOmegaDir reports omega's config folder itself.
func isOmegaDir(raw string) bool {
	rest, ok := underOmega(strings.ToLower(absForm(normalizeSecretPath(raw))))
	return ok && rest == ""
}

// secretDirs are folders whose files are secrets, as path suffixes.
var secretDirs = []string{"/.ssh", "/.aws", "/.gnupg", "/.local/share/keyrings", "/keychains"}

// secretDir reports a folder whose contents must not be printed.
func secretDir(raw string) bool {
	lower := strings.ToLower(absForm(normalizeSecretPath(raw)))
	if rest, ok := underOmega(lower); ok && rest == "" {
		return true
	}
	for _, d := range secretDirs {
		if strings.HasSuffix(lower, d) {
			return true
		}
	}
	return false
}

// holdsSecrets reports a folder above the secret ones: the home directory,
// its .config, the folders of every user and the root. Reading everything
// under one of them reads the keys too.
func holdsSecrets(raw string) bool {
	lower := strings.ToLower(absForm(normalizeSecretPath(raw)))
	home := strings.ToLower(homeSentinel)
	above := []string{"/", "/users", "/home", "/root", home, home + "/.config", home + "/.local", home + "/.local/share", home + "/library", xdgSentinel}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		h = strings.ToLower(path.Clean(strings.ReplaceAll(h, `\`, "/")))
		above = append(above, h, h+"/.config", h+"/.local", h+"/.local/share", h+"/library", path.Dir(h))
	}
	if x := configHome(); x != "" {
		above = append(above, strings.ToLower(path.Clean(strings.ReplaceAll(x, `\`, "/"))))
	}
	for _, a := range above {
		if lower == a {
			return true
		}
	}
	return anyHome.MatchString(lower)
}

// anyHome matches any user's home directory and the folders in it that
// hold configuration.
var anyHome = regexp.MustCompile(`^/(home|users)/[^/]+(/\.config|/\.local(/share)?|/library)?$`)

// absForm resolves a relative path against the working directory, so
// "keys" typed inside omega's folder is seen for what it is.
func absForm(p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "-") || driveLetter.MatchString(p) {
		return p
	}
	wd, err := getwd()
	if err != nil || wd == "" {
		return p
	}
	return path.Clean(strings.ReplaceAll(wd, `\`, "/") + "/" + p)
}

// driveLetter matches an absolute Windows path (C:/…).
var driveLetter = regexp.MustCompile(`^[A-Za-z]:/`)

var procEnviron = regexp.MustCompile(`^/proc/[^/]+/environ$`)

// SecretFile reports a path whose contents are a private key or a credential
// store. Those bytes must not be sent to the model. A public key, known_hosts
// and an SSH config are not secret. The directory itself is not either, so
// listing ~/.ssh stays allowed. A relative path is also checked against the
// working directory, and a glob against the files it could expand to.
func SecretFile(raw string) bool { return secretFile(raw, true) }

var number = regexp.MustCompile(`^[0-9.]+$`)

// secretFile is SecretFile; relative is false for a word that cannot be a
// path in the working directory (the program's own name).
func secretFile(raw string, relative bool) bool {
	p := normalizeSecretPath(raw)
	if p == "" || p == "." || strings.HasPrefix(path.Base(p), "-") {
		return false
	}
	if secretPath(p) {
		return true
	}
	// Inside omega's own folder a bare name ("keys", "./keys") is the key file.
	if abs := absForm(p); abs != p && relative && !number.MatchString(p) {
		if rest, ok := underOmega(strings.ToLower(abs)); ok && rest != "" && !omegaReadable[rest] {
			return true
		}
	}
	return globReachesKeys(absForm(p)) || globReachesSecrets(p)
}

// knownSecrets are the credential files with a fixed place under the home
// directory. A pattern is matched against them, since the shell expands it
// after the policy has looked: ~/.s*/id_*, ~/.aws/credential?.
var knownSecrets = []string{
	"/.ssh/id_rsa", "/.ssh/id_dsa", "/.ssh/id_ecdsa", "/.ssh/id_ed25519",
	"/.aws/credentials", "/.aws/config", "/.netrc", "/.npmrc", "/.pypirc", "/.env",
	"/.git-credentials", "/.docker/config.json", "/.kube/config", "/.config/gh/hosts.yml",
	"/.config/gcloud/application_default_credentials.json", "/.config/gcloud/credentials.db",
	"/.config/gcloud/access_tokens.db", "/.gnupg/private-keys-v1.d/key", "/.local/share/keyrings/login.keyring",
}

// globReachesSecrets reports a pattern that can expand to a credential
// file, or to any file of a folder that only holds secrets (a key in
// ~/.ssh may have any name).
func globReachesSecrets(p string) bool {
	if !strings.ContainsAny(p, "*?[{") {
		return false
	}
	lower := strings.ToLower(absForm(p))
	if strings.Contains(lower, "{") {
		// Braces are expanded by the shell, not by path.Match: read them
		// as "anything".
		lower = braces.ReplaceAllString(lower, "*")
	}
	homes := []string{strings.ToLower(homeSentinel)}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		homes = append(homes, strings.ToLower(path.Clean(strings.ReplaceAll(h, `\`, "/"))))
	}
	parts := strings.Split(lower, "/")
	for _, home := range homes {
		for _, k := range knownSecrets {
			if ok, err := path.Match(lower, home+k); err == nil && ok {
				return true
			}
		}
		for _, d := range secretDirs {
			dir := strings.Split(home+d, "/")
			if len(parts) <= len(dir) {
				continue
			}
			if ok, err := path.Match(strings.Join(parts[:len(dir)], "/"), home+d); err != nil || !ok {
				continue
			}
			rest := strings.Join(parts[len(dir):], "/")
			if strings.HasSuffix(rest, ".pub") {
				continue // public keys, whatever the pattern before the suffix
			}
			if strings.ContainsAny(rest, "*?[") || secretPath(home+d+"/"+rest) {
				return true
			}
		}
	}
	return false
}

var braces = regexp.MustCompile(`\{[^{}]*\}`)

// globReachesKeys reports a pattern (*, ?, [], {}) that can expand to
// omega's key file from outside its folder, such as ~/.config/*/keys.
func globReachesKeys(p string) bool {
	if !strings.ContainsAny(p, "*?[{") {
		return false
	}
	lower := strings.ToLower(p)
	keys := []string{strings.ToLower(homeSentinel) + "/.config/omega/keys", xdgSentinel + "/omega/keys"}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		keys = append(keys, strings.ToLower(path.Clean(strings.ReplaceAll(h, `\`, "/")))+"/.config/omega/keys")
	}
	if x := configHome(); x != "" {
		keys = append(keys, strings.ToLower(path.Clean(strings.ReplaceAll(x, `\`, "/")))+"/omega/keys")
	}
	for _, k := range keys {
		if ok, err := path.Match(lower, k); err == nil && ok {
			return true
		}
		// Braces are expanded by the shell, not by path.Match: a pattern
		// whose fixed start already points into the config folder is refused.
		if i := strings.IndexAny(lower, "{"); i >= 0 {
			if pre := lower[:i]; strings.HasPrefix(k, pre) && len(pre) >= len(path.Dir(path.Dir(k))) {
				return true
			}
		}
	}
	return false
}

func secretPath(p string) bool {
	base := strings.ToLower(path.Base(p))
	if strings.HasSuffix(base, ".pub") || base == "known_hosts" || base == "authorized_keys" || base == "environment" {
		return false
	}
	lower := strings.ToLower(p)
	if procEnviron.MatchString(lower) {
		// The environment of a process can hold an API key.
		return true
	}
	if strings.Contains(lower, "/.ssh/") || strings.HasPrefix(lower, ".ssh/") {
		return base != "config"
	}
	if strings.HasSuffix(lower, "/.docker/config.json") || strings.HasPrefix(lower, ".docker/config.json") ||
		strings.Contains(lower, "/gcloud/application_default_credentials.json") ||
		strings.HasSuffix(lower, "/gcloud/credentials.db") || strings.HasSuffix(lower, "/gcloud/access_tokens.db") {
		return true
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
	// omega's config folder holds the API key file on Linux: only its
	// configuration, notes and rules are readable, by exact name. The
	// GNOME keyring stores and GnuPG's private keys are never readable.
	if rest, ok := underOmega(lower); ok {
		return rest != "" && !omegaReadable[rest]
	}
	if strings.HasSuffix(lower, "/omega/keys") || strings.Contains(lower, "/.local/share/keyrings/") || strings.Contains(lower, "/.gnupg/private-keys") {
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
		".netrc", "_netrc", ".npmrc", ".pypirc", ".env", ".git-credentials":
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
	p = strings.ReplaceAll(p, "${XDG_CONFIG_HOME}", xdgSentinel)
	p = strings.ReplaceAll(p, "$XDG_CONFIG_HOME", xdgSentinel)
	if strings.HasPrefix(p, "~") {
		rest := strings.TrimPrefix(p, "~")
		if rest == "" || strings.HasPrefix(rest, "/") {
			p = homeSentinel + rest
		}
	}
	return path.Clean(p)
}

// shellSegments splits a command into its simple commands as loosely as a
// policy must: quotes are dropped, so the text handed to sh -c or eval is
// scanned like any other; command substitutions, pipes, lists, subshells and
// redirections all start a new segment; backslashes are removed. It
// over-approximates on purpose: a word that looks like a secret is treated
// as one wherever it appears.
func shellSegments(cmd string) [][]string { return segmentsWith(cmd, "") }

// segmentsWith is shellSegments with backslashes replaced by a string: ""
// reads them as shell escapes, "/" as the separators of a Windows path.
func segmentsWith(cmd, backslash string) [][]string {
	r := strings.NewReplacer("$(", " ; ", "`", " ; ", ";", " ; ", "|", " ; ", "&", " ; ", "(", " ; ", ")", " ; ",
		"<", " ; ", ">", " ; ", "\n", " ; ", "'", "", `"`, "", `\`, backslash)
	var out [][]string
	var cur []string
	for _, w := range strings.Fields(r.Replace(cmd)) {
		if w == ";" {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, w)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// wrapperArgs are the options of each wrapper that take a value.
var wrapperArgs = map[string]string{
	"sudo":  "-u -g -p -C -D -h -r -t -T -U",
	"env":   "-u -C -S",
	"nice":  "-n",
	"xargs": "-I -i -n -P -L -l -d -a -s -E -e",
	"time":  "-o -f",
}

// afterWrappers skips what runs another command (sudo, env, command, exec,
// nice, nohup, time, xargs, a shell with -c) and the words shells put before
// one (assignments, if, then, do, !), and returns the rest: its first word
// is the program that really runs.
func afterWrappers(words []string) []string {
	i := 0
	for i < len(words) {
		w := words[i]
		if strings.Contains(w, "=") && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "-") {
			i++
			continue
		}
		base := path.Base(w)
		switch base {
		case "sudo", "doas", "env", "command", "exec", "nice", "nohup", "time", "xargs", "builtin", "eval",
			"sh", "bash", "zsh", "dash", "ksh", "fish",
			"if", "then", "else", "elif", "do", "while", "until", "!", "{":
			i++
			takes := " " + wrapperArgs[base] + " "
			for i < len(words) && (strings.HasPrefix(words[i], "-") || (base == "env" && strings.Contains(words[i], "="))) {
				if strings.Contains(takes, " "+words[i]+" ") {
					i++
				}
				i++
			}
			continue
		}
		break
	}
	if i >= len(words) {
		return nil
	}
	return words[i:]
}

func commandVerb(words []string) string {
	if rest := afterWrappers(words); len(rest) > 0 {
		return rest[0]
	}
	return ""
}

// listingVerbs only print names and sizes: with them a secret directory
// itself may be named.
var listingVerbs = map[string]bool{"ls": true, "eza": true, "stat": true, "du": true, "df": true, "test": true, "[": true,
	"mkdir": true, "echo": true, "printf": true, "realpath": true, "file": true, "tree": true}

// recursiveReaders print the contents of every file under a directory.
func recursiveReader(words []string) bool {
	if len(words) == 0 {
		return false
	}
	switch path.Base(words[0]) {
	case "rg", "ag", "ack", "rgrep":
		return true
	case "grep", "egrep", "fgrep", "zgrep":
		for _, w := range words[1:] {
			if w == "--recursive" || w == "--dereference-recursive" || strings.HasPrefix(w, "--directories") {
				return true
			}
			if strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.ContainsAny(w, "rRd") {
				return true
			}
		}
	}
	return false
}

// leaksSecret reports a command that would print a private key, a credential
// file, omega's own key file, or a password kept in the Keychain or a Linux
// keyring. The whole command is blocked, including under --yes, because the
// output is sent to the model. It looks through wrappers (env, sudo, xargs,
// sh -c …), command substitutions and quoting.
func leaksSecret(cmd string) bool {
	if strings.Contains(cmd, `\`) && leaksIn(segmentsWith(cmd, "/")) {
		return true
	}
	return leaksIn(shellSegments(cmd))
}

func leaksIn(segs [][]string) bool {
	cdAbove, touchesOmega, xargs := false, false, false
	for _, seg := range segs {
		for _, w := range seg {
			if path.Base(w) == "xargs" {
				xargs = true
			}
		}
		run := afterWrappers(seg)
		verb := ""
		if len(run) > 0 {
			verb = path.Base(run[0])
		}
		for _, w := range seg {
			if path.Base(w) == "secret-tool" {
				// lookup and search print the secrets a Linux keyring holds.
				for _, x := range seg {
					if x == "lookup" || x == "search" {
						return true
					}
				}
			}
			if path.Base(w) == "security" && securityReveals(seg) {
				return true
			}
		}
		var targets []string
		for i, w := range seg {
			// The program's name is looked up on PATH, not in the folder.
			isVerb := len(run) > 0 && i == len(seg)-len(run)
			for _, cand := range wordPaths(w) {
				if secretFile(cand, !isVerb) {
					return true
				}
				if isOmegaDir(cand) {
					touchesOmega = true
					if !listingVerbs[verb] {
						return true
					}
				}
				if !strings.HasPrefix(cand, "-") {
					targets = append(targets, cand)
				}
			}
		}
		switch verb {
		case "cd", "pushd":
			for _, w := range run[1:] {
				if secretDir(w) {
					return true
				}
				if holdsSecrets(w) {
					cdAbove = true
				}
			}
		}
		if recursiveReader(run) {
			// With a pattern only, the search starts in the working directory.
			var roots []string
			for _, w := range run[1:] {
				if !strings.HasPrefix(w, "-") {
					roots = append(roots, w)
				}
			}
			if len(roots) < 2 {
				roots = append(roots, ".")
			}
			for _, root := range roots {
				if secretDir(root) || holdsSecrets(root) {
					return true
				}
			}
		}
	}
	if touchesOmega && xargs {
		return true
	}
	if cdAbove {
		// After a cd above omega's folder, a relative path can reach the keys.
		for _, seg := range segs {
			for _, w := range seg {
				for _, cand := range wordPaths(w) {
					rel := strings.ToLower(path.Clean(cand))
					for _, pre := range []string{"omega", ".config/omega", "config/omega"} {
						if rest, ok := strings.CutPrefix(rel, pre); ok && (rest == "" || strings.HasPrefix(rest, "/")) && !omegaReadable[rest] {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// wordPaths are the paths a shell word may carry: the word itself and, for
// VAR=path or --flag=path, what follows the equals sign.
func wordPaths(w string) []string {
	out := []string{w}
	if i := strings.LastIndex(w, "="); i >= 0 && i+1 < len(w) {
		out = append(out, w[i+1:])
	}
	return out
}

func securityReveals(words []string) bool {
	blob := strings.Join(words, " ")
	if strings.Contains(blob, "dump-keychain") || strings.Contains(blob, "dump-trust-settings") {
		return true
	}
	reveal := false
	for _, w := range words {
		if w == "--password" {
			reveal = true
		}
		// -w and -g print the password, alone or in a cluster (-wa, -ga).
		if strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "--") && strings.ContainsAny(w[1:], "wg") {
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
