// Package secscan audits SSH configuration and key permissions, scans shell
// history files for accidentally leaked credentials and API tokens, and reviews
// system privacy and telemetry settings.
package secscan

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var goos = runtime.GOOS

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
		info, err := e.Info()
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
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.EqualFold(parts[0], "IdentityFile") {
			keyPath := parts[1]
			if strings.HasPrefix(keyPath, "~/") {
				home, _ := os.UserHomeDir()
				keyPath = filepath.Join(home, strings.TrimPrefix(keyPath, "~/"))
			}
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				*findings = append(*findings, SSHFinding{
					Path:     path,
					Issue:    fmt.Sprintf("IdentityFile line %d refers to non-existent key '%s'", lineNum, parts[1]),
					Severity: "medium",
				})
			}
		}
	}
}

// Regex patterns for secrets
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"OpenAI API Key", regexp.MustCompile(`(?i)\b(sk-[a-zA-Z0-9]{32,64})\b`)},
	{"Anthropic API Key", regexp.MustCompile(`(?i)\b(sk-ant-[a-zA-Z0-9_\-]{32,100})\b`)},
	{"GitHub Personal Token", regexp.MustCompile(`\b(ghp_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82})\b`)},
	{"AWS Access Key", regexp.MustCompile(`\b(AKIA[0-9A-Z]{16})\b`)},
	{"Slack Token", regexp.MustCompile(`\b(xox[baprs]-[0-9a-zA-Z]{10,48})\b`)},
	{"Private Key Block", regexp.MustCompile(`-----BEGIN (RSA|EC|OPENSSH|DSA) PRIVATE KEY-----`)},
}

// ScanSecrets scans shell history files and dotfiles for leaked credentials.
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
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
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
	}

	return findings, nil
}

func maskSnippet(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 80 {
		line = line[:77] + "..."
	}
	for _, sp := range secretPatterns {
		line = sp.re.ReplaceAllString(line, "[REDACTED-SECRET]")
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

	// Telemetry findings
	if goos == "darwin" {
		audit.TelemetryFindings = append(audit.TelemetryFindings, "Apple Diagnostics & CrashReporter telemetry active (default on macOS)")
	} else if goos == "windows" {
		audit.TelemetryFindings = append(audit.TelemetryFindings, "Windows Telemetry & Customer Experience Improvement Program active")
	} else {
		if _, err := os.Stat("/etc/popularity-contest.conf"); err == nil {
			audit.TelemetryFindings = append(audit.TelemetryFindings, "Debian/Ubuntu popularity-contest package is present")
		}
	}

	return audit, nil
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
