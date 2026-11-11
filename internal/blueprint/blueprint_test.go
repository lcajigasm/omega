package blueprint

import (
	"context"
	"path/filepath"
	"testing"
)

func TestExportAndBootstrapSmoke(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "blueprint.json")

	bp, err := Export(context.Background(), outFile)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if bp == nil {
		t.Fatalf("expected non-nil SystemBlueprint")
	}
	if bp.Platform == "" {
		t.Errorf("expected platform to be set")
	}
}
