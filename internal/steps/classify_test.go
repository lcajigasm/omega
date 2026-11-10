package steps

import (
	"testing"

	"github.com/lcajigasm/omega/internal/safety"
)

// No catalogue step may be a command the safety policy refuses outright,
// and ids must be unique within a system: the agent runs steps by id.
func TestStepsAreRunnable(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		seen := map[string]bool{}
		for _, s := range For(goos) {
			if seen[s.ID] {
				t.Errorf("%s: duplicate step id %s", goos, s.ID)
			}
			seen[s.ID] = true
			if safety.Classify(s.Command) == safety.Blocked {
				t.Errorf("%s: the safety policy blocks %q", s.ID, s.Command)
			}
		}
	}
}
