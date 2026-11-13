package safety

import (
	"regexp"
	"strings"
)

// Rules for the commands omega runs on Windows, where the shell is
// PowerShell. They are matched on any system: a PowerShell command is the
// same threat wherever the policy happens to be asked about it.

// windowsBlocked never run.
var windowsBlocked = []*regexp.Regexp{
	// Downloading a script straight into the interpreter.
	regexp.MustCompile(`(?i)\b(iwr|irm|curl|wget|invoke-webrequest|invoke-restmethod)\b[^|]*\|\s*(iex|invoke-expression)\b`),
	regexp.MustCompile(`(?i)\b(iex|invoke-expression)\b.*\b(downloadstring|invoke-webrequest|invoke-restmethod|iwr|irm)\b`),
	// Wiping or repartitioning disks, and the boot configuration.
	regexp.MustCompile(`(?i)\b(format-volume|clear-disk|initialize-disk|remove-partition|diskpart|bcdedit)\b`),
	regexp.MustCompile(`(?i)(^|[;&|]\s*)format(\.com)?\s+[a-z]:`),
	regexp.MustCompile(`(?i)\bcipher(\.exe)?\s+/w`),
	// Throwing away the ways back: restore points, shadow copies, backups.
	regexp.MustCompile(`(?i)\bvssadmin(\.exe)?\s+delete\b|\bwbadmin(\.exe)?\s+delete\b|\bdisable-computerrestore\b`),
	// Switching off the system's own protections.
	regexp.MustCompile(`(?i)\bset-mppreference\b.*-disable\w+\s+(\$true|1)\b`),
	regexp.MustCompile(`(?i)\bnetsh\b.*\badvfirewall\b.*\bstate\s+off\b`),
	// Deleting the registry hives the system boots from.
	regexp.MustCompile(`(?i)\breg(\.exe)?\s+delete\s+"?(hklm|hkey_local_machine)\\(system|software|sam|security)\b"?(\s|$)`),
	// The provider keys, wherever Windows keeps them: the Credential
	// Manager, the user's environment in the registry, or the variable.
	regexp.MustCompile(`(?i)\b(passwordvault|credread|credenumerate|get-storedcredential|vaultcmd)\b`),
	regexp.MustCompile(`(?i)(anthropic|openai|openrouter|opencode)_api_key`),
	regexp.MustCompile(`(?i)hkcu:?\\environment|hkey_current_user\\environment`),
}

func init() { blocked = append(blocked, windowsBlocked...) }

// psRemove are the ways PowerShell and cmd delete things.
var psRemove = regexp.MustCompile(`(?i)^(remove-item|ri|rm|rmdir|rd|del|erase)$`)

// psWipeTarget is a whole drive, the user profile, or a directory Windows
// itself lives in. A path underneath one of them is not.
var psWipeTarget = regexp.MustCompile(`(?i)^(` +
	`[a-z]:[\\/]*` + // C:, C:\
	`|[a-z]:[\\/]+(windows|users|program files|program files \(x86\)|programdata)[\\/]*` +
	`|~[\\/]*|\$home[\\/]*` +
	`|\$env:(systemroot|windir|systemdrive|userprofile|homepath|programfiles|programdata|appdata|localappdata)[\\/]*` +
	`|%(systemroot|windir|systemdrive|userprofile|programfiles|programdata|appdata|localappdata)%[\\/]*` +
	`)(\*|\*\.\*)?$`)

// windowsWipes reports a delete aimed at one of those roots.
func windowsWipes(part string) bool {
	words := psWords(part)
	if len(words) < 2 || !psRemove.MatchString(words[0]) {
		return false
	}
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "-") || (strings.HasPrefix(w, "/") && len(w) <= 3) {
			continue // a parameter name, or a cmd switch such as /s /q
		}
		for _, target := range strings.Split(w, ",") {
			if psWipeTarget.MatchString(strings.Trim(target, `"'`)) {
				return true
			}
		}
	}
	return false
}

// psWords splits a PowerShell command into words, keeping quoted paths with
// spaces ("C:\Program Files") together.
func psWords(part string) []string {
	var out []string
	var cur strings.Builder
	quote := rune(0)
	for _, r := range strings.TrimSpace(part) {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// psReadOnly are cmdlets that only look: the verbs PowerShell reserves for
// reading and shaping output. Out-File and the Set/New/Remove families are
// not here, so they stay dangerous.
var psReadOnly = regexp.MustCompile(`(?i)^((get|test|measure|select|where|sort|format|group|compare|resolve|convertto|convertfrom|find|show|read|search)-[a-z]+|out-(string|null|host|default)|write-(output|host)|foreach-object|%|\?)$`)

// psSimpleBlock is a script block that only compares or picks a property
// of the current object: { $_.Length -gt 1MB }, { $_.Name }.
var psSimpleBlock = regexp.MustCompile(`^\s*\$_(\.\w+)*(\s+-[a-z]+\s+("[^"$]*"|'[^']*'|[\w.]+))?\s*$`)

var psBlock = regexp.MustCompile(`\{([^{}]*)\}`)

// psRunsCode reports a reading cmdlet that is handed code to run: only its
// first word says "read". ForEach-Object { Remove-Item $_ } deletes, and
// so does a method call such as $_.Delete() or [IO.File]::Delete(...).
func psRunsCode(p string) bool {
	if strings.Contains(p, "::") || strings.Contains(p, "(") {
		return true
	}
	if strings.Count(p, "{") != len(psBlock.FindAllString(p, -1)) {
		return true // nested or unbalanced blocks
	}
	for _, m := range psBlock.FindAllStringSubmatch(p, -1) {
		if !psSimpleBlock.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// psWrites are parameters that turn a reading cmdlet into a writing one.
var psWrites = regexp.MustCompile(`(?i)\s-(outfile|outvariable|filepath|literalpath\s+\S+\s+-value|passthru\s+\|\s*set-)\b`)

// Read-only Windows commands that are not cmdlets.
var windowsSafePrefixes = []string{
	"winget list", "winget search", "winget show", "winget source list", "winget --version", "winget --info",
	"scoop status", "scoop list", "scoop info", "scoop search", "scoop bucket list", "scoop which", "scoop --version",
	"choco outdated", "choco list", "choco info", "choco search", "choco --version",
	"systeminfo", "tasklist", "ver", "where", "where.exe", "dir", "ipconfig", "wsl --list", "wsl -l", "wsl --status", "wsl --version",
	"dism /online /get-", "dism.exe /online /get-", "sfc /verifyonly", "powercfg /a", "powercfg /list", "powercfg /energy",
	"cmdkey /list", "driverquery", "net use", "net start", "sc query", "sc.exe query", "schtasks /query",
}

// windowsNormalPrefixes are routine updates of software the user installed
// for themselves, without elevation.
var windowsNormalPrefixes = []string{"scoop update"}

// wingetLists matches "winget upgrade" with nothing to upgrade named: it
// only lists what could be.
var wingetLists = regexp.MustCompile(`(?i)^winget(\.exe)?\s+(upgrade|update)(\s+--(include-unknown|include-pinned|accept-source-agreements|disable-interactivity|source\s+\S+))*\s*$`)

// classifyWindows decides a single PowerShell or Windows command. ok is
// false when the command is not one this file knows.
func classifyWindows(p string) (Risk, bool) {
	words := psWords(p)
	if len(words) == 0 {
		return Safe, false
	}
	if psReadOnly.MatchString(words[0]) {
		if psWrites.MatchString(p) || psRunsCode(p) {
			return Dangerous, true
		}
		return Safe, true
	}
	if wingetLists.MatchString(p) {
		return Safe, true
	}
	lower := strings.ToLower(p)
	for _, pre := range windowsSafePrefixes {
		if hasPrefixWord(lower, pre) {
			return Safe, true
		}
	}
	for _, pre := range windowsNormalPrefixes {
		if hasPrefixWord(lower, pre) {
			return Normal, true
		}
	}
	return Safe, false
}
