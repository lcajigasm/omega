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

// KeptSecretVars are the credentials that stay in the environment of the
// commands omega runs, because the tools it looks after need them: brew and
// gh read the GitHub tokens, npm its registry token. Every other variable
// that looks like a credential is taken out (see the runner). A command
// that names one of these is refused: it could print the value in pieces
// that no mask on the output would recognise.
var KeptSecretVars = []string{"HOMEBREW_GITHUB_API_TOKEN", "HOMEBREW_DOCKER_REGISTRY_TOKEN", "HOMEBREW_DOCKER_REGISTRY_BASIC_AUTH_TOKEN",
	"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GITHUB_API_TOKEN", "MISE_GITHUB_TOKEN",
	"NPM_TOKEN", "NODE_AUTH_TOKEN"}

var keptSecret = regexp.MustCompile(`(?i)\b(` + strings.Join(KeptSecretVars, "|") + `)\b`)

// namesKeptSecret reports a command that names one of KeptSecretVars:
// $NAME, ${NAME:0:10}, printenv NAME, $env:NAME.
func namesKeptSecret(cmd string) bool { return keptSecret.MatchString(cmd) }

// secretFileVar are the variables whose value is the path of a credential
// file or of a folder of them. They stay in the commands' environment, so
// "cat $KUBECONFIG" reads the file without ever naming it.
var secretFileVar = regexp.MustCompile(`(?i)\$(env:)?\{?(GOOGLE_APPLICATION_CREDENTIALS|KUBECONFIG|AWS_SHARED_CREDENTIALS_FILE|AWS_CONFIG_FILE|` +
	`DOCKER_CONFIG|NETRC|HISTFILE|PGPASSFILE|PGSERVICEFILE|NPM_CONFIG_USERCONFIG|BOTO_CONFIG|CLOUDSDK_CONFIG|AZURE_CONFIG_DIR|` +
	`VAULT_TOKEN_FILE|SOPS_AGE_KEY_FILE|GNUPGHOME|RCLONE_CONFIG)\b`)

func namesSecretFileVar(cmd string) bool { return secretFileVar.MatchString(cmd) }

// homes are the home directory as the policy writes it and as it is.
func homes() []string {
	out := []string{strings.ToLower(homeSentinel)}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		out = append(out, strings.ToLower(path.Clean(strings.ReplaceAll(h, `\`, "/"))))
	}
	return out
}

// containsSecret reports a folder with a known credential file somewhere
// below it (~/.config/gh, ~/.docker, ~/.claude): reading every file under
// it reads that one.
func containsSecret(raw string) bool {
	lower := strings.ToLower(absForm(normalizeSecretPath(raw)))
	for _, home := range homes() {
		for _, k := range knownSecrets {
			if strings.HasPrefix(home+k, strings.TrimSuffix(lower, "/")+"/") {
				return true
			}
		}
	}
	return false
}

// emptyVar is a variable written inside a word. One that is not set expands
// to nothing: ~/.ss${Z}h/id_r${Z}sa is ~/.ssh/id_rsa.
var emptyVar = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// looseRegexp reads a shell pattern generously, as the shells do and
// path.Match does not: [!x] and [[:alpha:]] are any character, **/ is any
// number of folders, {a,b} anything at all.
func looseRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if strings.HasPrefix(pattern[i:], "**/") {
				b.WriteString("(.*/)?")
				i += 2
			} else if strings.HasPrefix(pattern[i:], "**") {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[min(i+2, len(pattern)):], ']')
			if end < 0 {
				b.WriteString(regexp.QuoteMeta("["))
				continue
			}
			i = min(i+2, len(pattern)) + end
			for i+1 < len(pattern) && pattern[i+1] == ']' {
				i++ // the end of [[:alpha:]]
			}
			b.WriteString("[^/]+")
		case '{':
			end := strings.IndexByte(pattern[i:], '}')
			if end < 0 {
				b.WriteString(regexp.QuoteMeta("{"))
				continue
			}
			i += end
			b.WriteString(".*")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("/?$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

// looselyReaches reports a word that the shell can turn into a credential
// file, a folder of them or one of extra (paths under the home directory),
// through a pattern or a variable that is not set.
func looselyReaches(p string, extra ...string) bool {
	if !strings.ContainsAny(p, "*?[{$") {
		return false
	}
	lower := strings.ToLower(absForm(emptyVar.ReplaceAllString(p, "")))
	if lower == "" || strings.HasSuffix(lower, ".pub") {
		return false
	}
	re := looseRegexp(lower)
	if re == nil {
		return false
	}
	var targets []string
	for _, home := range homes() {
		for _, e := range extra {
			targets = append(targets, home+e)
		}
		if len(extra) > 0 {
			continue
		}
		for _, k := range knownSecrets {
			targets = append(targets, home+k)
		}
		for _, d := range secretDirs {
			targets = append(targets, home+d, home+d+"/id_key")
		}
		targets = append(targets, home+"/.config/omega", home+"/.config/omega/keys")
	}
	if len(extra) == 0 {
		targets = append(targets, xdgSentinel+"/omega", xdgSentinel+"/omega/keys", xdgSentinel+"/gh/hosts.yml")
		if x := configHome(); x != "" {
			x = strings.ToLower(path.Clean(strings.ReplaceAll(x, `\`, "/")))
			targets = append(targets, x+"/omega", x+"/omega/keys", x+"/gh/hosts.yml")
		}
	}
	for _, t := range targets {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

// startupFiles are the files a shell reads when it starts. Most people
// export their API keys there.
var startupFiles = []string{".zshrc", ".zshenv", ".zprofile", ".zlogin", ".bashrc", ".bash_profile", ".bash_login", ".profile",
	".zshrc.local", ".bash_aliases", ".envrc", ".config/fish/config.fish"}

var startupName = regexp.MustCompile(`(?i)(^|[/\s<=:])(\.(zshrc|zshenv|zprofile|zlogin|bashrc|bash_profile|bash_login|profile|bash_aliases|envrc)(\.local)?|config\.fish|\S*profile\.ps1)($|[\s;|&<>)])`)

// wholeLines are the programs that print the lines of a file as they are,
// or nothing of them: the mask on the output recognises NAME=value there.
var wholeLines = map[string]bool{"cat": true, "grep": true, "egrep": true, "fgrep": true, "wc": true, "ls": true, "stat": true,
	"head": true, "tail": true, "sort": true, "uniq": true, "less": true, "more": true, "file": true, "test": true, "[": true,
	"echo": true, "printf": true, "nl": true, "cd": true, "true": true, "du": true, "eza": true, "bat": true}

// startupFileReshaped reports a command that reads a shell startup file
// with something that can print its lines in pieces: cut -c, awk substr,
// grep -o, tail -c. The keys exported there are masked in the output as
// whole NAME=value words; cut up, no mask recognises them.
func startupFileReshaped(code string) bool {
	for _, pipeline := range splitUnquoted(code, false, true) {
		plain := dequote(pipeline)
		named := startupName.MatchString(plain)
		if !named {
			for _, w := range strings.Fields(plain) {
				if looselyReaches(normalizeSecretPath(w), prefixed(startupFiles)...) {
					named = true
					break
				}
			}
		}
		if !named {
			continue
		}
		for _, stage := range splitUnquoted(pipeline, true, false) {
			f := strings.Fields(joinText(shellWords(stage)))
			if len(f) == 0 {
				continue
			}
			if !wholeLines[path.Base(f[0])] {
				return true
			}
			for _, a := range f[1:] {
				switch path.Base(f[0]) {
				case "grep", "egrep", "fgrep":
					if abbrev(a, "--only-matching") || (shortFlags.MatchString(a) && strings.Contains(a, "o")) {
						return true
					}
				case "head", "tail":
					if abbrev(a, "--bytes") || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c")) {
						return true
					}
				}
			}
		}
	}
	return false
}

func prefixed(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "/" + n
	}
	return out
}

// secretDirs are folders whose files are secrets, as path suffixes.
var secretDirs = []string{"/.ssh", "/.aws", "/.gnupg", "/.local/share/keyrings", "/keychains",
	"/.azure", "/.kube", "/.config/gcloud", "/library/cookies"}

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
	return looselyReaches(normalizeSecretPath(raw))
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
	// A variable that is not set vanishes from the word.
	if bare := emptyVar.ReplaceAllString(p, ""); bare != p && bare != "" && bare != "/" && secretPath(path.Clean(bare)) {
		return true
	}
	if looselyReaches(p) {
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
	"/.zsh_history", "/.bash_history", "/.pgpass", "/.my.cnf", "/.cargo/credentials", "/.cargo/credentials.toml",
	"/.claude/.credentials.json", "/.claude.json", "/.codex/auth.json", "/.local/share/opencode/auth.json",
	"/.config/rclone/rclone.conf", "/.terraform.d/credentials.tfrc.json", "/.vault-token", "/.huggingface/token",
	"/.cache/huggingface/token", "/.gradle/gradle.properties", "/.m2/settings.xml", "/.m2/settings-security.xml",
	"/.config/sops/age/keys.txt", "/.age/key.txt", "/.s3cfg", "/.boto", "/.config/doctl/config.yaml", "/.vercel/auth.json",
	"/.fly/config.yml", "/.config/configstore/firebase-tools.json", "/.composer/auth.json", "/.wrangler/config/default.toml",
	"/.yarnrc.yml", "/.config/hub", "/.wakatime.cfg", "/.cursor/mcp.json", "/.gnupg/secring.gpg",
	"/library/application support/claude/claude_desktop_config.json", "/.zsh_sessions/session.history", "/.bash_sessions/session.history",
}

// moreSecrets are those of knownSecrets that secretPath does not already
// know by a rule of its own, as path suffixes.
var moreSecrets = []string{"/.config/sops/age/keys.txt", "/.age/key.txt", "/.s3cfg", "/.boto", "/doctl/config.yaml", "/.vercel/auth.json",
	"/.fly/config.yml", "/configstore/firebase-tools.json", "/.composer/auth.json", "/.wrangler/config/default.toml",
	"/.yarnrc.yml", "/.config/hub", "/.wakatime.cfg", "/.cursor/mcp.json", "/.gnupg/secring.gpg", "/claude_desktop_config.json",
	"/gh/hosts.yml"}

// omegaBackup is the suffix of the copy write_file keeps of the file it
// replaces: the copy of a secret is as secret.
var omegaBackup = regexp.MustCompile(`\.omega-bak-[0-9-]+$`)

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
	p = omegaBackup.ReplaceAllString(p, "")
	base := strings.ToLower(path.Base(p))
	if strings.HasSuffix(base, ".pub") || base == "known_hosts" || base == "authorized_keys" || base == "environment" {
		return false
	}
	lower := strings.ToLower(p)
	for _, f := range moreSecrets {
		if strings.HasSuffix(lower, f) || lower == f[1:] {
			return true
		}
	}
	if strings.Contains(lower, "/.zsh_sessions/") || strings.Contains(lower, "/.bash_sessions/") {
		return true
	}
	if procEnviron.MatchString(lower) {
		// The environment of a process can hold an API key.
		return true
	}
	if strings.Contains(lower, "/.ssh/") || strings.HasPrefix(lower, ".ssh/") {
		return base != "config"
	}
	switch lower {
	case "/etc/shadow", "/etc/gshadow", "/etc/master.passwd", "/private/etc/master.passwd":
		return true
	}
	// Folders where every file is a token or a cache of one.
	for _, dir := range []string{"/.aws/", "/.azure/", "/.kube/", "/.config/gcloud/", "/library/cookies/"} {
		if strings.Contains(lower, dir) {
			return true
		}
	}
	for _, file := range []string{"/.codex/auth.json", "/opencode/auth.json", "huggingface/token", "/.gradle/gradle.properties",
		"/.m2/settings.xml", "/.m2/settings-security.xml", "/.cargo/credentials.toml"} {
		if strings.HasSuffix(lower, file) {
			return true
		}
	}
	// A browser keeps its sessions and saved passwords in the profile
	// folder. "Login Data" reaches here whole from read_file and as its
	// first half from a command line, where the blank splits it.
	if browserProfile.MatchString(lower) {
		return true
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

// browserProfile matches the cookie and password stores of a browser.
var browserProfile = regexp.MustCompile(`/(default|network|profile[^/]*)/(cookies|login data|login)$|/(cookies\.sqlite|cookies\.binarycookies|logins\.json|key4\.db|login data)$`)

func secretBasename(base string) bool {
	switch base {
	case "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
		".netrc", "_netrc", ".npmrc", ".pypirc", ".env", ".git-credentials",
		// What was typed in a shell or a database client, passwords included.
		".histfile", "consolehost_history.txt",
		".pgpass", ".my.cnf", ".vault-token", ".credentials.json", ".claude.json", "rclone.conf",
		"credentials.tfrc.json", ".terraformrc":
		return true
	}
	if strings.HasSuffix(base, "_history") {
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
	for _, v := range []string{"$env:USERPROFILE", "$env:HOME", "${env:USERPROFILE}", "%USERPROFILE%"} {
		if i := strings.Index(strings.ToLower(p), strings.ToLower(v)); i >= 0 {
			p = p[:i] + homeSentinel + p[i+len(v):]
		}
	}
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
	if listsIntoReader(cmd) {
		return true
	}
	if strings.Contains(cmd, `\`) && leaksIn(segmentsWith(cmd, "/")) {
		return true
	}
	return leaksIn(shellSegments(cmd))
}

// contentVerbs print what is in the files they are handed.
var contentVerbs = map[string]bool{"cat": true, "get-content": true, "gc": true, "type": true, "select-string": true, "sls": true,
	"more": true, "less": true, "head": true, "tail": true, "xargs": true, "foreach-object": true, "%": true, "strings": true}

// listsIntoReader reports a secret folder listed into a later stage of
// the same pipeline that prints the files: Get-ChildItem ~\.ssh |
// Get-Content names no key file, and neither does ls ~/.ssh | xargs cat.
func listsIntoReader(cmd string) bool {
	for _, pipeline := range splitUnquoted(cmd, false, true) {
		listed := false // by an earlier stage
		for _, stage := range splitUnquoted(pipeline, true, false) {
			lists := false
			for _, seg := range segmentsWith(stage, "/") {
				run := afterWrappers(seg)
				if len(run) == 0 {
					continue
				}
				verb := strings.ToLower(path.Base(run[0]))
				if listed && contentVerbs[verb] {
					return true
				}
				if has("ls eza tree find dir get-childitem gci get-item gi", verb) {
					for _, w := range run[1:] {
						lists = lists || secretDir(w) || containsSecret(w)
					}
				}
			}
			listed = listed || lists
		}
	}
	return false
}

func leaksIn(segs [][]string) bool {
	cdAbove, touchesOmega, xargs := false, false, false
	dir := "" // where an earlier cd left the shell
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
				// After "cd ~/.docker", config.json is the credential file.
				if dir != "" && !isVerb && verb != "echo" && verb != "printf" && relativeWord(cand) && secretFile(dir+"/"+cand, false) {
					return true
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
			dir = cdTarget(dir, run[1:])
		}
		// find that prints what it finds reads every file below its roots.
		if verb == "find" && findPrints(run[1:]) {
			roots := 0
			for _, w := range run[1:] {
				if strings.HasPrefix(w, "-") {
					break
				}
				roots++
				if secretDir(w) || holdsSecrets(w) || containsSecret(w) || (dir != "" && relativeWord(w) && unsafeRoot(dir+"/"+w)) {
					return true
				}
			}
			if roots == 0 && (unsafeRoot(".") || (dir != "" && unsafeRoot(dir))) {
				return true
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
				if unsafeRoot(root) || (dir != "" && relativeWord(root) && unsafeRoot(dir+"/"+root)) {
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

// unsafeRoot reports a folder whose files must not all be printed: one that
// only holds secrets, one above those, or one with a credential file in it.
func unsafeRoot(root string) bool {
	return secretDir(root) || holdsSecrets(root) || containsSecret(root)
}

// relativeWord reports a word that can be a path under the working directory.
func relativeWord(w string) bool {
	return w != "" && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, "~") && !strings.HasPrefix(w, "$") &&
		!strings.HasPrefix(w, "-") && !driveLetter.MatchString(w)
}

// findPrints reports a find that hands its files to a program that prints
// their contents.
func findPrints(args []string) bool {
	for i, a := range args {
		if (a == "-exec" || a == "-execdir") && i+1 < len(args) && has("head cat tail less more grep", path.Base(args[i+1])) {
			return true
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
