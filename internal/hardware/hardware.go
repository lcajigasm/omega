// Package hardware provides diagnostics for battery health, NVMe/SSD wear,
// runaway process detection and RAM purging.
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

// run executes a shell command. Tests replace it with canned output.
var run = func(ctx context.Context, command string, timeout time.Duration) runner.Result {
	return runner.Run(ctx, command, timeout, nil)
}

// powerSupplyDir is where Linux describes batteries.
var powerSupplyDir = "/sys/class/power_supply"

// BatteryHealth holds parsed battery diagnostics.
type BatteryHealth struct {
	Supported      bool    `json:"supported"`
	Present        bool    `json:"present"`
	Condition      string  `json:"condition"` // e.g. "Normal", "Good", "Service Battery"
	CycleCount     int     `json:"cycle_count"`
	MaxCapacityPct float64 `json:"max_capacity_pct"` // % of design capacity
	// Not every system reports the two above. When these are false the
	// numbers are zero because nothing was read, not because they are zero.
	CyclesKnown   bool    `json:"cycles_known"`
	CapacityKnown bool    `json:"capacity_known"`
	CurrentPct    int     `json:"current_pct"`
	IsCharging    bool    `json:"is_charging"`
	TemperatureC  float64 `json:"temperature_c,omitempty"`
	Raw           string  `json:"raw,omitempty"`
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

// unknown is the condition of a battery nothing could be read about.
const unknown = "Unknown"

// conditionFor is a word for the battery's state where the system gives
// none: 80% of the design capacity is where makers call a battery worn.
func conditionFor(h *BatteryHealth) string {
	switch {
	case !h.CapacityKnown:
		return unknown
	case h.MaxCapacityPct >= 80:
		return "Good"
	}
	return "Degraded"
}

// HealthText is the battery's health in words: "Normal (91.0% max
// capacity)", and "unknown" for what the system did not report.
func (h *BatteryHealth) HealthText() string {
	switch {
	case h.CapacityKnown:
		return fmt.Sprintf("%s (%.1f%% max capacity)", h.Condition, h.MaxCapacityPct)
	case h.Condition != "" && h.Condition != unknown:
		return h.Condition + " (max capacity unknown)"
	}
	return "unknown (this system does not report it)"
}

// Text is the battery in one line: charge, condition, cycles and health,
// with "unknown" for what the system did not report.
func (h *BatteryHealth) Text() string {
	cycles, health := "cycles unknown", "health unknown"
	if h.CyclesKnown {
		cycles = fmt.Sprintf("%d cycles", h.CycleCount)
	}
	if h.CapacityKnown {
		health = fmt.Sprintf("%.1f%% health", h.MaxCapacityPct)
	}
	parts := []string{cycles, health}
	if h.Condition != "" && h.Condition != unknown {
		parts = append([]string{h.Condition}, parts...)
	}
	return fmt.Sprintf("%d%% (%s)", h.CurrentPct, strings.Join(parts, ", "))
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
	res := run(ctx, "system_profiler SPPowerDataType 2>/dev/null", 5*time.Second)
	if res.ExitCode != 0 || len(strings.TrimSpace(res.Output)) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}
	out := res.Output
	if !strings.Contains(out, "Battery Information") {
		return &BatteryHealth{Supported: false}, nil
	}

	h := &BatteryHealth{Supported: true, Present: true, Condition: unknown}

	// Extract Cycle Count
	reCycle := regexp.MustCompile(`(?i)Cycle Count:\s*(\d+)`)
	if m := reCycle.FindStringSubmatch(out); len(m) > 1 {
		h.CycleCount, _ = strconv.Atoi(m[1])
		h.CyclesKnown = true
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
		h.CapacityKnown = true // Intel Macs do not print this line
	}

	// Extract State of Charge
	reCharge := regexp.MustCompile(`(?i)State of Charge \(%\):\s*(\d+)`)
	if m := reCharge.FindStringSubmatch(out); len(m) > 1 {
		h.CurrentPct, _ = strconv.Atoi(m[1])
	}

	// Charging?
	reCharging := regexp.MustCompile(`(?i)Charging:\s*(Yes|True)`)
	h.IsCharging = reCharging.MatchString(out)
	if h.Condition == unknown {
		h.Condition = conditionFor(h)
	}

	h.Raw = out
	return h, nil
}

func getBatteryLinux(ctx context.Context) (*BatteryHealth, error) {
	// Check /sys/class/power_supply/BAT0 or similar
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil || len(entries) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}

	var batDir string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "BAT") {
			batDir = powerSupplyDir + "/" + e.Name()
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

	h := &BatteryHealth{Supported: true, Present: true}
	status := readSys("status")
	h.IsCharging = status == "Charging"

	capStr := readSys("capacity")
	h.CurrentPct, _ = strconv.Atoi(capStr)

	// Some batteries report energy (µWh) and others charge (µAh). With
	// neither the capacity is unknown, not 100%.
	for _, unit := range []string{"energy", "charge"} {
		full, _ := strconv.ParseFloat(readSys(unit+"_full"), 64)
		design, _ := strconv.ParseFloat(readSys(unit+"_full_design"), 64)
		if design > 0 && full > 0 {
			h.MaxCapacityPct = (full / design) * 100
			h.CapacityKnown = true
			break
		}
	}
	h.Condition = conditionFor(h)

	// Drivers that do not count cycles write 0 or nothing.
	if n, err := strconv.Atoi(readSys("cycle_count")); err == nil && n > 0 {
		h.CycleCount, h.CyclesKnown = n, true
	}

	return h, nil
}

// windowsBatteryScript prints what Windows knows about the battery, one
// "Name : value" per line, and nothing when there is none. Charge comes
// from Win32_Battery; capacity and cycles from the root\wmi classes, which
// not every machine has: a value that is missing is printed empty.
const windowsBatteryScript = `$b = Get-CimInstance Win32_Battery | Select-Object -First 1; if ($b) { ` +
	`$wmi = { param($class, $prop) (Get-CimInstance -Namespace root\wmi -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1).$prop }; ` +
	`"EstimatedChargeRemaining : $($b.EstimatedChargeRemaining)"; "BatteryStatus : $($b.BatteryStatus)"; ` +
	`"FullChargedCapacity : $(& $wmi BatteryFullChargedCapacity FullChargedCapacity)"; ` +
	`"DesignedCapacity : $(& $wmi BatteryStaticData DesignedCapacity)"; ` +
	`"CycleCount : $(& $wmi BatteryCycleCount CycleCount)" }`

func getBatteryWindows(ctx context.Context) (*BatteryHealth, error) {
	res := run(ctx, windowsBatteryScript, 10*time.Second)
	if res.ExitCode != 0 || len(strings.TrimSpace(res.Output)) == 0 {
		return &BatteryHealth{Supported: false}, nil
	}

	// A number after a name, or false when the value is missing.
	number := func(name string) (float64, bool) {
		m := regexp.MustCompile(`(?im)^\s*` + name + `\s*:\s*(\d+)\s*$`).FindStringSubmatch(res.Output)
		if m == nil {
			return 0, false
		}
		v, err := strconv.ParseFloat(m[1], 64)
		return v, err == nil
	}
	h := &BatteryHealth{Supported: true, Present: true}
	if v, ok := number("EstimatedChargeRemaining"); ok {
		h.CurrentPct = int(v)
	}
	// 2 is "on AC power"; 6 to 9 are the charging states.
	if v, ok := number("BatteryStatus"); ok {
		h.IsCharging = v == 2 || (v >= 6 && v <= 9)
	}
	full, okFull := number("FullChargedCapacity")
	design, okDesign := number("DesignedCapacity")
	if okFull && okDesign && full > 0 && design > 0 {
		h.MaxCapacityPct = full / design * 100
		h.CapacityKnown = true
	}
	if v, ok := number("CycleCount"); ok && v > 0 {
		h.CycleCount, h.CyclesKnown = int(v), true
	}
	h.Condition = conditionFor(h)
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

	res := run(ctx, cmdStr, 5*time.Second)
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
	res := run(ctx, script, 5*time.Second)
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
		res := run(ctx, "sudo purge", 10*time.Second)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("purge failed (requires sudo): %s", res.Output)
		}
		return "macOS inactive memory cache purged successfully", nil
	case "windows":
		// Windows clear memory cache via PowerShell working set trim
		script := `[System.GC]::Collect(); [System.GC]::WaitForPendingFinalizers()`
		res := run(ctx, script, 5*time.Second)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("windows memory cleanup failed: %s", res.Output)
		}
		return "Windows garbage collection and memory cache released", nil
	default: // Linux
		res := run(ctx, "sudo sync; echo 3 | sudo tee /proc/sys/vm/drop_caches", 10*time.Second)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("linux drop_caches failed: %s", res.Output)
		}
		return "Linux pagecache, dentries and inodes purged successfully", nil
	}
}
