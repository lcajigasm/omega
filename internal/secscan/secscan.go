// Package secscan audits SSH configuration and key permissions, scans shell
// history files for accidentally leaked credentials and API tokens, and reviews
// system privacy and telemetry settings.
package secscan

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	{"Slack Token", regexp.MustCompile(`\b(xox[baprs]-[0-9a-zA-Z]{10,48})\b`)},
	{"Private Key Block", regexp.MustCompile(`-----BEGIN (RSA|EC|OPENSSH|DSA) PRIVATE KEY-----`)},
}

// ScanSecrets scans shell history files and dotfiles for leaked credentials.
// A file that could not be read to its end is an error: "no secrets found"
// would not be true of it.
func ScanSecrets() ([]SecretFinding, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	targets := []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".bash_history"),
		filepath.Join(home, ".history"),
		filepath.Join(home, ".sh_history"),
		filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"),
	}

	var findings []SecretFinding
	for _, file := range targets {
		found, err := scanFile(file)
		findings = append(findings, found...)
		if err != nil {
			return findings, fmt.Errorf("%s was not read to the end: %w", file, err)
		}
	}
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

// maskSnippet is a line made safe to show: the secrets are taken out first
// and the line is shortened after. The other way round a key cut by the
// shortening no longer matches its pattern and is shown.
func maskSnippet(line string) string {
	line = strings.TrimSpace(line)
	for _, sp := range secretPatterns {
		line = sp.re.ReplaceAllString(line, "[REDACTED-SECRET]")
	}
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
		const where = "System Settings › Privacy & Security › Analytics & Improvements"
		out, err := output("/usr/bin/defaults", "read", "/Library/Application Support/CrashReporter/DiagnosticMessagesHistory.plist", "AutoSubmit")
		switch strings.TrimSpace(string(out)) {
		case "1":
			return []string{"Share Mac Analytics is on: diagnostics and usage data are sent to Apple (" + where + ")"}
		case "0":
			if err == nil {
				return []string{"Share Mac Analytics is off (" + where + ")"}
			}
		}
		return []string{"The Mac Analytics setting could not be read; see " + where}
	case "windows":
		return []string{"Windows diagnostic data is not checked by omega; see Settings › Privacy & security › Diagnostics & feedback"}
	}
	data, err := os.ReadFile(popconFile)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok && key == "PARTICIPATE" {
			if strings.EqualFold(strings.Trim(value, `"' `), "yes") {
				return []string{"popularity-contest is on: the list of installed packages is sent to the distribution"}
			}
			return []string{"popularity-contest is installed and switched off"}
		}
	}
	return []string{"popularity-contest is installed; its setting could not be read from " + popconFile}
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
