package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/devtools"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/runner"
	"github.com/lcajigasm/omega/internal/safety"
)

// system_info, dev_inventory and disk_usage: what the machine is and
// where its disk went.

// systemInfoCommand prints the system summary of an operating system.
func systemInfoCommand(goos string) string {
	if goos == "darwin" {
		return `sw_vers; echo "arch: $(uname -m)"; echo "model: $(sysctl -n hw.model 2>/dev/null)"; echo; ` +
			`df -h /System/Volumes/Data 2>/dev/null | tail -1 || df -h / | tail -1; echo; ` +
			`sysctl -n hw.memsize | awk '{printf "RAM: %.0f GB\n", $1/1073741824}'; uptime; ` +
			`echo "developer tools: $(xcode-select -p 2>/dev/null || echo none)"; ` +
			`[ -d /Applications/Xcode.app ] && echo "Xcode.app: installed" || echo "Xcode.app: not installed"`
	}
	if goos == "windows" {
		return `$os = Get-CimInstance Win32_OperatingSystem; "{0} {1} (build {2})" -f $os.Caption, $os.Version, $os.BuildNumber; ` +
			`"arch: $env:PROCESSOR_ARCHITECTURE"; "powershell: $($PSVersionTable.PSVersion)"; ` +
			`"elevated: " + ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator); ` +
			`"restart pending: " + (Test-Path 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired'); ""; ` +
			`Get-Volume | Where-Object { $_.DriveLetter } | ForEach-Object { "{0}: {1:N1} GB free of {2:N1} GB" -f $_.DriveLetter, ($_.SizeRemaining/1GB), ($_.Size/1GB) }; ""; ` +
			`"RAM: {0:N0} GB" -f ($os.TotalVisibleMemorySize/1MB); "up since: " + $os.LastBootUpTime`
	}
	return `grep -E '^(PRETTY_NAME|ID|ID_LIKE|VERSION_ID)=' /etc/os-release 2>/dev/null; echo "kernel: $(uname -sr)"; echo "arch: $(uname -m)"; ` +
		`[ -d /run/systemd/system ] && echo "init: systemd"; echo "desktop: ${XDG_CURRENT_DESKTOP:-none}"; ` +
		`[ -f /var/run/reboot-required ] && echo "restart pending: yes (/var/run/reboot-required)"; echo; df -h / | tail -1; echo; ` +
		`awk '/^MemTotal/ {printf "RAM: %.0f GB\n", $2/1048576}' /proc/meminfo; uptime`
}

// detectable are the package managers and tools system_info looks for.
func detectable(goos string) []string {
	common := []string{"brew", "npm", "pnpm", "bun", "pipx", "uv", "pip3", "cargo", "rustup", "gem", "go", "gh", "docker", "topgrade"}
	if goos == "darwin" {
		return append(common, "mas", "xcrun", "mo")
	}
	if goos == "windows" {
		return []string{"winget", "scoop", "choco", "npm", "pnpm", "bun", "pipx", "uv", "pip", "cargo", "rustup", "gem", "go", "gh", "docker", "wsl", "topgrade"}
	}
	return append([]string{"apt-get", "dnf", "pacman", "zypper", "flatpak", "snap"}, common...)
}

func toolSystemInfo(ctx context.Context, a *Agent, _ map[string]any) (string, error) {
	a.emit(Event{Kind: EvToolStart, Tool: "system_info", Command: "system_info"})
	res := a.opts.Exec(ctx, systemInfoCommand(goos), 30*time.Second, nil)
	var managers []string
	for _, bin := range detectable(goos) {
		if runner.Has(bin) {
			managers = append(managers, bin)
		}
	}
	a.emit(Event{Kind: EvToolEnd, Tool: "system_info", Command: "system_info", ExitCode: res.ExitCode})
	var b strings.Builder
	b.WriteString(strings.TrimSpace(res.Output))
	fmt.Fprintf(&b, "\n\ndetected tools: %s\n", strings.Join(managers, ", "))
	fmt.Fprintf(&b, "shell: %s, omega on %s/%s\n", runner.Shell(), goos, runtime.GOARCH)
	if total, free, ok := diskSpace(dataVolume()); ok {
		// One figure for the whole run: df on "/" reports the sealed
		// system volume on macOS, Mole counts in decimal GB, and runs that
		// mixed them contradicted themselves.
		fmt.Fprintf(&b, "FREE SPACE: %s free of %s on %s (decimal GB). Quote this figure, measured the same way, before and after cleaning.\n",
			gb(free), gb(total), dataVolume())
	}
	configs := []string{"~/.config/topgrade.toml"}
	if goos == "darwin" {
		configs = append(configs, "~/.config/mole")
	}
	for _, f := range configs {
		if _, err := os.Stat(expand(f)); err == nil {
			fmt.Fprintf(&b, "exists: %s\n", f)
		}
	}
	return b.String(), nil
}

func toolDevInventory(_ context.Context, a *Agent, _ map[string]any) (string, error) {
	a.emit(Event{Kind: EvToolStart, Tool: "dev_inventory", Command: "dev_inventory", Risk: safety.Safe})
	start := time.Now()
	report := devInventory()
	a.emit(Event{Kind: EvToolEnd, Tool: "dev_inventory", Command: "dev_inventory", Duration: time.Since(start), Lines: strings.Count(report, "\n")})
	return report, nil
}

// devInventory is replaced by tests and by the demo, which must not inspect
// the machine they run on.
var devInventory = func() string { return devtools.Load(true).Report() }

func toolDiskUsage(ctx context.Context, a *Agent, in map[string]any) (string, error) {
	path := expand(getString(in, "path"))
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	top := 15
	if n, ok := in["top"].(float64); ok && n > 0 && n <= 100 {
		top = int(n)
	}
	// du exits with 1 when a folder in the tree cannot be read, which on
	// macOS is always; under pipefail that made the whole scan a failure
	// that was not cached and ran again. What it measured is still right.
	cmd := fmt.Sprintf("{ du -xhd 1 %s 2>/dev/null || true; } | sort -hr | head -n %d", safety.ShellQuote(path), top+1)
	if goos == "windows" {
		cmd = diskUsageWindows(path, top)
	}
	return a.runShell(ctx, "disk_usage", cmd, i18n.T("agent.disk", path), safety.Safe, false)
}

// diskUsageWindows is disk_usage in PowerShell: the size of each entry of a
// folder, largest first. The path goes in single quotes, where PowerShell
// expands nothing.
func diskUsageWindows(path string, top int) string {
	quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	return fmt.Sprintf("Get-ChildItem -LiteralPath %s -Force -ErrorAction SilentlyContinue | ForEach-Object { "+
		"$s = $_.Length; if ($_.PSIsContainer) { $s = (Get-ChildItem -LiteralPath $_.FullName -Recurse -Force -ErrorAction SilentlyContinue | Measure-Object Length -Sum).Sum }; "+
		"[pscustomobject]@{ GB = [math]::Round($s/1GB, 2); Name = $_.Name } } | Sort-Object GB -Descending | Select-Object -First %d | Format-Table -AutoSize | Out-String -Width 160", quoted, top)
}
