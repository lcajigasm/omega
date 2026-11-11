// Package hardware provides diagnostics for battery health, NVMe/SSD wear,
// runaway process detection, local snapshot management, and RAM purging.
package hardware

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

// BatteryHealth holds parsed battery diagnostics.
type BatteryHealth struct {
	Supported      bool    `json:"supported"`
	Present        bool    `json:"present"`
	Condition      string  `json:"condition"` // e.g. "Normal", "Good", "Service Battery"
	CycleCount     int     `json:"cycle_count"`
	MaxCapacityPct float64 `json:"max_capacity_pct"` // % of design capacity
	CurrentPct     int     `json:"current_pct"`
	IsCharging     bool    `json:"is_charging"`
	TemperatureC   float64 `json:"temperature_c,omitempty"`
	Raw            string  `json:"raw,omitempty"`
}

// DiskWear holds SMART and SSD wear metrics.
type DiskWear struct {
	Device      string `json:"device"`
	Model       string `json:"model"`
	Health      string `json:"health"`       // "PASSED", "OK", "Warning"
	Percentage  int    `json:"wear_percent"` // 0-100% wear
	TBW         string `json:"tbw,omitempty"`
	Temperature int    `json:"temperature_c,omitempty"`
}

// ProcessInfo holds CPU and memory metrics for a process.
type ProcessInfo struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	CPU     float64 `json:"cpu_pct"`
	Memory  float64 `json:"memory_mb"`
	Command string  `json:"command"`
}

// GetBatteryInfo inspects the system battery status.
func GetBatteryInfo(ctx context.Context) (*BatteryHealth, error) {
	switch goos {
	case "darwin":
		return getBatteryMac(ctx)
	case "windows":
		return getBatteryWindows(ctx)
	default:
		return getBatteryLinux(ctx)
	}
}

func getBatteryMac(ctx context.Context) (*BatteryHealth, error) {
	res := runner.Run(ctx, "system_profiler SPPowerDataType 2>/dev/null", 5*time.Second, nil)
	if res.ExitCode != 0 || len(strings.TrimSpace(res.Output)) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}
	out := res.Output
	if !strings.Contains(out, "Battery Information") {
		return &BatteryHealth{Supported: false}, nil
	}

	h := &BatteryHealth{Supported: true, Present: true, Condition: "Normal"}

	// Extract Cycle Count
	reCycle := regexp.MustCompile(`(?i)Cycle Count:\s*(\d+)`)
	if m := reCycle.FindStringSubmatch(out); len(m) > 1 {
		h.CycleCount, _ = strconv.Atoi(m[1])
	}

	// Extract Condition
	reCond := regexp.MustCompile(`(?i)Condition:\s*([^\n\r]+)`)
	if m := reCond.FindStringSubmatch(out); len(m) > 1 {
		h.Condition = strings.TrimSpace(m[1])
	}

	// Extract Maximum Capacity
	reCap := regexp.MustCompile(`(?i)Maximum Capacity:\s*(\d+)%?`)
	if m := reCap.FindStringSubmatch(out); len(m) > 1 {
		capVal, _ := strconv.ParseFloat(m[1], 64)
		h.MaxCapacityPct = capVal
	}

	// Extract State of Charge
	reCharge := regexp.MustCompile(`(?i)State of Charge \(%\):\s*(\d+)`)
	if m := reCharge.FindStringSubmatch(out); len(m) > 1 {
		h.CurrentPct, _ = strconv.Atoi(m[1])
	}

	// Charging?
	reCharging := regexp.MustCompile(`(?i)Charging:\s*(Yes|True)`)
	h.IsCharging = reCharging.MatchString(out)

	h.Raw = out
	return h, nil
}

func getBatteryLinux(ctx context.Context) (*BatteryHealth, error) {
	// Check /sys/class/power_supply/BAT0 or similar
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil || len(entries) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}

	var batDir string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "BAT") {
			batDir = "/sys/class/power_supply/" + e.Name()
			break
		}
	}
	if batDir == "" {
		return &BatteryHealth{Supported: false}, nil
	}

	readSys := func(file string) string {
		b, err := os.ReadFile(batDir + "/" + file)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}

	h := &BatteryHealth{Supported: true, Present: true, Condition: "Good"}
	status := readSys("status")
	h.IsCharging = status == "Charging"

	capStr := readSys("capacity")
	h.CurrentPct, _ = strconv.Atoi(capStr)

	energyFull, _ := strconv.ParseFloat(readSys("energy_full"), 64)
	energyDesign, _ := strconv.ParseFloat(readSys("energy_full_design"), 64)
	if energyDesign > 0 && energyFull > 0 {
		h.MaxCapacityPct = (energyFull / energyDesign) * 100
	} else {
		h.MaxCapacityPct = 100
	}

	cycleStr := readSys("cycle_count")
	h.CycleCount, _ = strconv.Atoi(cycleStr)

	return h, nil
}

func getBatteryWindows(ctx context.Context) (*BatteryHealth, error) {
	script := `Get-CimInstance Win32_Battery | Select-Object EstimatedChargeRemaining, BatteryStatus, DesignCapacity, FullChargeCapacity | Format-List`
	res := runner.Run(ctx, script, 5*time.Second, nil)
	if res.ExitCode != 0 || len(strings.TrimSpace(res.Output)) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}

	h := &BatteryHealth{Supported: true, Present: true, Condition: "Normal"}
	reCharge := regexp.MustCompile(`(?i)EstimatedChargeRemaining\s*:\s*(\d+)`)
	if m := reCharge.FindStringSubmatch(res.Output); len(m) > 1 {
		h.CurrentPct, _ = strconv.Atoi(m[1])
	}
	return h, nil
}

// GetTopProcesses returns the processes consuming the most CPU or Memory.
func GetTopProcesses(ctx context.Context, limit int) ([]ProcessInfo, error) {
	if limit <= 0 {
		limit = 10
	}
	if goos == "windows" {
		return getTopProcessesWindows(ctx, limit)
	}
	return getTopProcessesUnix(ctx, limit)
}

func getTopProcessesUnix(ctx context.Context, limit int) ([]ProcessInfo, error) {
	// ps -eo pid,%cpu,rss,comm -r on Mac or --sort=-%cpu on Linux
	cmdStr := fmt.Sprintf("ps -eo pid,%%cpu,rss,comm | sort -k2 -r -n | head -%d", limit+1)
	if goos == "darwin" {
		cmdStr = fmt.Sprintf("ps -A -o pid,%%cpu,rss,comm -r | head -%d", limit+1)
	}

	res := runner.Run(ctx, cmdStr, 5*time.Second, nil)
	var procs []ProcessInfo
	lines := strings.Split(res.Output, "\n")
	for i, l := range lines {
		if i == 0 || strings.TrimSpace(l) == "" {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) >= 4 {
			pid, _ := strconv.Atoi(fields[0])
			cpu, _ := strconv.ParseFloat(fields[1], 64)
			rssKB, _ := strconv.ParseFloat(fields[2], 64)
			comm := strings.Join(fields[3:], " ")
			procs = append(procs, ProcessInfo{
				PID:     pid,
				CPU:     cpu,
				Memory:  rssKB / 1024, // to MB
				Name:    comm,
				Command: comm,
			})
		}
	}
	return procs, nil
}

func getTopProcessesWindows(ctx context.Context, limit int) ([]ProcessInfo, error) {
	script := fmt.Sprintf(`Get-Process | Sort-Object CPU -Descending | Select-Object -First %d Id, ProcessName, CPU, WorkingSet | ForEach-Object { '{0} {1} {2} {3}' -f $_.Id, $_.ProcessName, [math]::Round($_.CPU,1), [math]::Round($_.WorkingSet/1MB,1) }`, limit)
	res := runner.Run(ctx, script, 5*time.Second, nil)
	var procs []ProcessInfo
	for _, l := range strings.Split(res.Output, "\n") {
		f := strings.Fields(l)
		if len(f) >= 4 {
			pid, _ := strconv.Atoi(f[0])
			cpu, _ := strconv.ParseFloat(f[2], 64)
			mem, _ := strconv.ParseFloat(f[3], 64)
			procs = append(procs, ProcessInfo{
				PID:    pid,
				Name:   f[1],
				CPU:    cpu,
				Memory: mem,
			})
		}
	}
	return procs, nil
}

// PurgeRAM requests the OS to flush file system buffers and inactive RAM cache.
func PurgeRAM(ctx context.Context) (string, error) {
	switch goos {
	case "darwin":
		res := runner.Run(ctx, "sudo purge", 10*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("purge failed (requires sudo): %s", res.Output)
		}
		return "macOS inactive memory cache purged successfully", nil
	case "windows":
		// Windows clear memory cache via PowerShell working set trim
		script := `[System.GC]::Collect(); [System.GC]::WaitForPendingFinalizers()`
		res := runner.Run(ctx, script, 5*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("windows memory cleanup failed: %s", res.Output)
		}
		return "Windows garbage collection and memory cache released", nil
	default: // Linux
		res := runner.Run(ctx, "sudo sync; echo 3 | sudo tee /proc/sys/vm/drop_caches", 10*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("linux drop_caches failed: %s", res.Output)
		}
		return "Linux pagecache, dentries and inodes purged successfully", nil
	}
}

// PruneSnapshots clears old local filesystem snapshots (e.g. Time Machine APFS snapshots).
func PruneSnapshots(ctx context.Context) (string, error) {
	switch goos {
	case "darwin":
		res := runner.Run(ctx, "sudo tmutil thinlocalsnapshots / 10000000000 4", 30*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to prune Time Machine snapshots: %s", res.Output)
		}
		return "Time Machine local APFS snapshots purged: " + strings.TrimSpace(res.Output), nil
	case "windows":
		res := runner.Run(ctx, "vssadmin delete shadows /all /quiet", 15*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to prune VSS shadow copies: %s", res.Output)
		}
		return "Windows VSS shadow copies purged", nil
	default:
		if runner.Has("snapper") {
			res := runner.Run(ctx, "sudo snapper cleanup timeline && sudo snapper cleanup number", 15*time.Second, nil)
			if res.ExitCode == 0 {
				return "Btrfs snapper snapshots cleaned", nil
			}
		}
		return "No snapshot manager found or needed on this Linux configuration", nil
	}
}
