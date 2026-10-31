package steps

import (
	"runtime"
	"testing"
)

func TestMacOSStepsFollowTheOS(t *testing.T) {
	for _, id := range []string{"macos", "macos-check"} {
		s, ok := Find(id)
		if !ok {
			t.Fatalf("missing step %s", id)
		}
		if s.Available() != (runtime.GOOS == "darwin") {
			t.Fatalf("%s available=%v on %s", id, s.Available(), runtime.GOOS)
		}
	}
}
