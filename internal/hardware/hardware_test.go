package hardware

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// answer makes every command print out.
func answer(t *testing.T, system, out string) {
	t.Helper()
	oldOS, oldRun := goos, run
	t.Cleanup(func() { goos, run = oldOS, oldRun })
	goos = system
	run = func(context.Context, string, time.Duration) runner.Result { return runner.Result{Output: out} }
}

// Windows often reports the charge and nothing else: the capacity and the
// cycles are then unknown, not "0.0% max capacity" and "0 cycles".
func TestWindowsBatteryUnknownIsNotZero(t *testing.T) {
	ctx := context.Background()
	answer(t, "windows", "EstimatedChargeRemaining : 87\nBatteryStatus : 2\nFullChargedCapacity : \nDesignedCapacity : \nCycleCount : \n")
	h, err := GetBatteryInfo(ctx)
	if err != nil || !h.Supported || h.CurrentPct != 87 || h.CapacityKnown || h.CyclesKnown || !h.IsCharging {
		t.Fatalf("battery: %+v, %v", h, err)
	}
	for _, text := range []string{h.HealthText(), h.Text()} {
		if strings.Contains(text, "0.0%") || strings.Contains(text, "0 cycles") || strings.Contains(text, "Normal") || !strings.Contains(text, "unknown") {
			t.Errorf("an unknown value is shown as a number or as fine: %q", text)
		}
	}

	answer(t, "windows", "EstimatedChargeRemaining : 50\nBatteryStatus : 1\nFullChargedCapacity : 45000\nDesignedCapacity : 50000\nCycleCount : 312\n")
	h, _ = GetBatteryInfo(ctx)
	if !h.CapacityKnown || h.MaxCapacityPct != 90 || !h.CyclesKnown || h.CycleCount != 312 || h.IsCharging {
		t.Fatalf("battery: %+v", h)
	}
	if got := h.HealthText(); got != "Good (90.0% max capacity)" {
		t.Errorf("HealthText = %q", got)
	}
	if got := h.Text(); got != "50% (Good, 312 cycles, 90.0% health)" {
		t.Errorf("Text = %q", got)
	}

	answer(t, "windows", "")
	if h, _ := GetBatteryInfo(ctx); h.Supported {
		t.Errorf("a machine with no battery: %+v", h)
	}
}

func TestMacBattery(t *testing.T) {
	ctx := context.Background()
	answer(t, "darwin", "Power:\n    Battery Information:\n      Charge Information:\n          State of Charge (%): 76\n"+
		"      Health Information:\n          Cycle Count: 120\n          Condition: Normal\n          Maximum Capacity: 91%\n")
	h, _ := GetBatteryInfo(ctx)
	if got := h.Text(); got != "76% (Normal, 120 cycles, 91.0% health)" {
		t.Errorf("Text = %q (%+v)", got, h)
	}
	// An Intel Mac prints no maximum capacity.
	answer(t, "darwin", "Battery Information:\n  State of Charge (%): 76\n  Cycle Count: 120\n  Condition: Normal\n")
	h, _ = GetBatteryInfo(ctx)
	if got := h.HealthText(); got != "Normal (max capacity unknown)" {
		t.Errorf("HealthText = %q", got)
	}
}

// A Linux battery that reports charge instead of energy, or neither.
func TestLinuxBattery(t *testing.T) {
	old := powerSupplyDir
	defer func() { powerSupplyDir = old }()
	powerSupplyDir = t.TempDir()
	answer(t, "linux", "")
	bat := filepath.Join(powerSupplyDir, "BAT0")
	os.MkdirAll(bat, 0o755)
	os.WriteFile(filepath.Join(bat, "capacity"), []byte("64\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "status"), []byte("Discharging\n"), 0o644)
	h, _ := GetBatteryInfo(context.Background())
	if !h.Supported || h.CapacityKnown || h.CyclesKnown || strings.Contains(h.HealthText(), "100") {
		t.Fatalf("nothing but the charge is known: %+v, %q", h, h.HealthText())
	}
	os.WriteFile(filepath.Join(bat, "charge_full"), []byte("4000000\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "charge_full_design"), []byte("5000000\n"), 0o644)
	os.WriteFile(filepath.Join(bat, "cycle_count"), []byte("0\n"), 0o644)
	h, _ = GetBatteryInfo(context.Background())
	if got := h.Text(); got != "64% (Good, cycles unknown, 80.0% health)" {
		t.Errorf("Text = %q", got)
	}
}

func TestGetBatteryInfo(t *testing.T) {
	ctx := context.Background()
	info, err := GetBatteryInfo(ctx)
	if err != nil {
		t.Fatalf("GetBatteryInfo returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatalf("expected non-nil battery info")
	}
}

func TestGetTopProcesses(t *testing.T) {
	ctx := context.Background()
	procs, err := GetTopProcesses(ctx, 5)
	if err != nil {
		t.Fatalf("GetTopProcesses returned error: %v", err)
	}
	if len(procs) == 0 {
		t.Log("Note: no processes parsed (could be CI environment)")
	}
}
