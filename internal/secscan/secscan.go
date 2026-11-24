// Package secscan audits SSH configuration and key permissions, scans shell
// history files for accidentally leaked credentials and API tokens, and reviews
// system privacy and telemetry settings.
package secscan

import (
	"bufio"
	"fmt"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

var goos = runtime.GOOS

// output runs a read-only command and returns what it printed. Tests
// replace it.
var output = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// popconFile is where Debian and Ubuntu keep the popularity-contest answer.
var popconFile = "/etc/popularity-contest.conf"

// SSHFinding describes an issue detected in SSH configuration or keys.
type SSHFinding struct {
	Path     string `json:"path"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"` // "high", "medium", "low"
	Fix      string `json:"fix,omitempty"`
}

// SecretFinding describes a secret pattern found in shell history or dotfiles.
type SecretFinding struct {
	File    string `json:"file"`
	LineNum int    `json:"line_num"`
	Type    string `json:"type"`
	Snippet string `json:"snippet"`
}

// BrowserCache holds information about detected browser cache sizes.
type BrowserCache struct {
	Browser string `json:"browser"`
	Path    string `json:"path"`
	SizeMB  int64  `json:"size_mb"`
}

// PrivacyAudit holds telemetry status and browser cache findings.
type PrivacyAudit struct {
	TelemetryFindings []string       `json:"telemetry_findings"`
	BrowserCaches     []BrowserCache `json:"browser_caches"`
}

// AuditSSH inspects ~/.ssh directory permissions and key files.
func AuditSSH() ([]SSHFinding, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")
	fi, err := os.Stat(sshDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No .ssh dir
		}
		return nil, err
	}

	var findings []SSHFinding

	// Check .ssh dir permissions (Unix only)
	if goos != "windows" {
		mode := fi.Mode().Perm()
		if mode&0077 != 0 {
			findings = append(findings, SSHFinding{
				Path:     sshDir,
				Issue:    fmt.Sprintf("Directory permissions too open (%04o); should be 0700", mode),
				Severity: "high",
				Fix:      fmt.Sprintf("chmod 700 %s", sshDir),
			})
		}
	}

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		if os.IsPermission(err) || strings.Contains(err.Error(), "operation not permitted") {
			findings = append(findings, SSHFinding{
				Path:     sshDir,
				Issue:    "Cannot inspect ~/.ssh directory (permission denied or sandboxed)",
				Severity: "low",
			})
			return findings, nil
		}
		return findings, err
	}

	for _, e := range entries {
		path := filepath.Join(sshDir, e.Name())
		// Stat, not the entry's own information: for a key that is a link
		// (to a dotfiles folder, say) ssh looks at the file it points to,
		// and the link itself always reads as open to everyone.
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		name := e.Name()
		mode := info.Mode().Perm()

		// Private keys check
		if isPrivateKey(name) && goos != "windows" {
			if mode&0077 != 0 {
				findings = append(findings, SSHFinding{
					Path:     path,
					Issue:    fmt.Sprintf("Private key has insecure permissions (%04o); should be 0600", mode),
					Severity: "high",
					Fix:      fmt.Sprintf("chmod 600 %s", path),
				})
			}
		}

		// Config file check
		if name == "config" {
			auditSSHConfigFile(path, &findings)
		}
	}

	return findings, nil
}

func isPrivateKey(name string) bool {
	// Standard private key names
	if strings.HasPrefix(name, "id_") && !strings.HasSuffix(name, ".pub") {
		return true
	}
	if strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key") {
		return true
	}
	return false
}

// identityFile reads an IdentityFile line of an ssh config and returns the
// path it names. ok is false for any other line, and for a value whose file
// cannot be known from here: "none", or one with %h, %u or ${VAR} in it.
func identityFile(line string) (path string, ok bool) {
	line = strings.TrimSpace(line)
	i := strings.IndexAny(line, " \t=")
	if i < 0 || !strings.EqualFold(line[:i], "IdentityFile") {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimLeft(line[i:], " \t="))
	// ssh takes the value in double quotes too, as it must be with spaces.
	if len(value) >= 2 && value[0] == '"' {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			value = value[1 : 1+end]
		}
	} else if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}
	if value == "" || strings.EqualFold(value, "none") || strings.ContainsAny(value, "%$") {
		return "", false
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		value = filepath.Join(home, strings.TrimPrefix(value, "~"))
	}
	return value, true
}

func auditSSHConfigFile(path string, findings *[]SSHFinding) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keyPath, ok := identityFile(line)
		if !ok {
			continue
		}
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			*findings = append(*findings, SSHFinding{
				Path:     path,
				Issue:    fmt.Sprintf("IdentityFile line %d refers to non-existent key '%s'", lineNum, keyPath),
				Severity: "medium",
			})
		}
	}
}

// Regex patterns for secrets
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	// Project, service-account and admin keys have a prefix and may hold
	// "-" and "_"; the older ones are letters and digits only.
	{"OpenAI API Key", regexp.MustCompile(`(?i)\b(sk-(?:proj|svcacct|admin)-[a-zA-Z0-9_\-]{20,}|sk-[a-zA-Z0-9]{32,64}\b)`)},
	{"Anthropic API Key", regexp.MustCompile(`(?i)\b(sk-ant-[a-zA-Z0-9_\-]{32,100})\b`)},
	{"GitHub Personal Token", regexp.MustCompile(`\b(ghp_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82})\b`)},
	{"AWS Access Key", regexp.MustCompile(`\b(AKIA[0-9A-Z]{16})\b`)},
	{"Slack Token", regexp.MustCompile(`\b(xox[abeprs]-[0-9a-zA-Z-]{10,})`)},
	{"Private Key Block", regexp.MustCompile(`-----BEGIN (RSA|EC|OPENSSH|DSA) PRIVATE KEY-----`)},
}

// SecretScan is what audit secrets read and found.
type SecretScan struct {
	Scanned  []string        `json:"scanned"` // the files read
	Findings []SecretFinding `json:"findings"`
}

// historyFiles are the command histories looked through: the shells'
// (zsh, bash, sh, fish, PowerShell), atuin's database, and the REPLs where
// keys get pasted (python, node, psql, mysql).
func historyFiles(home string) (text, binary []string) {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	text = []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".bash_history"),
		filepath.Join(home, ".history"),
		filepath.Join(home, ".sh_history"),
		filepath.Join(data, "fish", "fish_history"),
		filepath.Join(home, ".python_history"),
		filepath.Join(home, ".node_repl_history"),
		filepath.Join(home, ".psql_history"),
		filepath.Join(home, ".mysql_history"),
		filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"),
	}
	// atuin keeps its history in an SQLite file; the commands are stored
	// as plain text inside it.
	binary = []string{filepath.Join(data, "atuin", "history.db")}
	return text, binary
}

// ScanSecrets scans the command histories (see historyFiles) for leaked
// credentials. A file that could not be read to its end is an error: "no
// secrets found" would not be true of it.
func ScanSecrets() ([]SecretFinding, error) {
	scan, err := ScanHistories()
	return scan.Findings, err
}

// ScanHistories is ScanSecrets, also saying which files it read.
func ScanHistories() (SecretScan, error) {
	scan := SecretScan{Scanned: []string{}, Findings: []SecretFinding{}}
	home, err := os.UserHomeDir()
	if err != nil {
		return scan, err
	}
	text, binary := historyFiles(home)
	for _, file := range append(text, binary...) {
		if fi, err := os.Stat(file); err != nil || fi.IsDir() {
			continue
		}
		var found []SecretFinding
		if slices.Contains(binary, file) {
			found, err = scanBinary(file)
		} else {
			found, err = scanFile(file)
		}
		scan.Scanned = append(scan.Scanned, file)
		scan.Findings = append(scan.Findings, found...)
		if err != nil {
			return scan, fmt.Errorf("%s was not read to the end: %w", file, err)
		}
	}
	return scan, nil
}

// scanBinary looks for secrets in the runs of printable text of a file
// that is not text (a database). LineNum is 0: lines mean nothing there.
func scanBinary(file string) ([]SecretFinding, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var findings []SecretFinding
	seen := map[string]bool{}
	start := -1
	flush := func(end int) {
		if start < 0 || end-start < 8 {
			start = -1
			return
		}
		run := string(data[start:end])
		start = -1
		for _, sp := range secretPatterns {
			if sp.re.MatchString(run) {
				snippet := maskSnippet(run)
				if !seen[snippet] { // atuin keeps a command more than once
					seen[snippet] = true
					findings = append(findings, SecretFinding{File: file, Type: sp.name, Snippet: snippet})
				}
				break
			}
		}
	}
	for i, b := range data {
		if b >= 0x20 && b < 0x7f || b == '\t' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(data))
	return findings, nil
}

// scanFile looks for secrets in one file. A file that is not there has none.
func scanFile(file string) ([]SecretFinding, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var findings []SecretFinding
	// Not a Scanner: it gives up on a line over 64 KB (a pasted file, a
	// here-document), and everything after it would go unread.
	r := bufio.NewReader(f)
	for lineNum := 1; ; lineNum++ {
		line, err := r.ReadString('\n')
		if line != "" {
			for _, sp := range secretPatterns {
				if sp.re.MatchString(line) {
					findings = append(findings, SecretFinding{
						File:    file,
						LineNum: lineNum,
						Type:    sp.name,
						Snippet: maskSnippet(line),
					})
					break
				}
			}
		}
		if err == io.EOF {
			return findings, nil
		}
		if err != nil {
			return findings, err
		}
	}
}

var userPass = regexp.MustCompile(`((?:^|\s)(?:-u|--user)[ =]?[^\s:]+:)\S+`)

// maskSnippet is a line made safe to show: the secrets are taken out first
// and the line is shortened after. The other way round a key cut by the
// shortening no longer matches its pattern and is shown.
func maskSnippet(line string) string {
	line = strings.TrimSpace(line)
	for _, sp := range secretPatterns {
		line = sp.re.ReplaceAllString(line, "[REDACTED-SECRET]")
	}
	// And whatever else on the line is a credential: the password of
	// "curl -u user:password", of a URL, a NAME=value next to the key found.
	line = userPass.ReplaceAllString(runner.Scrub(line), "${1}[REDACTED-SECRET]")
	if r := []rune(line); len(r) > 80 {
		line = string(r[:77]) + "..."
	}
	return line
}

// AuditPrivacy inspects system telemetry and browser caches.
func AuditPrivacy() (*PrivacyAudit, error) {
	audit := &PrivacyAudit{}
	home, _ := os.UserHomeDir()

	// Browser caches
	var candidateDirs = []struct {
		browser string
		path    string
	}{
		{"Google Chrome", filepath.Join(home, "Library/Caches/Google/Chrome")},
		{"Google Chrome (Linux)", filepath.Join(home, ".cache/google-chrome")},
		{"Brave Browser", filepath.Join(home, "Library/Caches/BraveSoftware/Brave-Browser")},
		{"Arc Browser", filepath.Join(home, "Library/Caches/company.thebrowser.Browser")},
		{"Mozilla Firefox", filepath.Join(home, "Library/Caches/Firefox")},
		{"Mozilla Firefox (Linux)", filepath.Join(home, ".cache/mozilla/firefox")},
		{"Apple Safari", filepath.Join(home, "Library/Caches/com.apple.Safari")},
	}

	for _, c := range candidateDirs {
		if fi, err := os.Stat(c.path); err == nil && fi.IsDir() {
			size := dirSizeMB(c.path)
			if size > 1 {
				audit.BrowserCaches = append(audit.BrowserCaches, BrowserCache{
					Browser: c.browser,
					Path:    c.path,
					SizeMB:  size,
				})
			}
		}
	}

	audit.TelemetryFindings = telemetry()

	return audit, nil
}

// telemetry says what the system's diagnostics setting is where one read
// gives it, and says so where it is not looked at. It never states a
// setting it did not read.
func telemetry() []string {
	switch goos {
	case "darwin":
		where := i18n.T("privacy.mac_where")
		out, err := output("/usr/bin/defaults", "read", "/Library/Application Support/CrashReporter/DiagnosticMessagesHistory.plist", "AutoSubmit")
		switch strings.TrimSpace(string(out)) {
		case "1":
			return []string{i18n.T("privacy.mac_on", where)}
		case "0":
			if err == nil {
				return []string{i18n.T("privacy.mac_off", where)}
			}
		}
		return []string{i18n.T("privacy.mac_unknown", where)}
	case "windows":
		return []string{i18n.T("privacy.windows")}
	}
	data, err := os.ReadFile(popconFile)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "PARTICIPATE" {
			if strings.EqualFold(strings.Trim(value, `"' `), "yes") {
				return []string{i18n.T("privacy.popcon_on")}
			}
			return []string{i18n.T("privacy.popcon_off")}
		}
	}
	return []string{i18n.T("privacy.popcon_unknown", popconFile)}
}

func dirSizeMB(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total / (1024 * 1024)
}
