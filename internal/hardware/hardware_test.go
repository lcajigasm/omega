package hardware

import (
	"context"
	"testing"
)

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
