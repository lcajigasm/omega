package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/lcajigasm/omega/internal/config"
)

// customRecord is one run of one step by `omega custom`: when, how long,
// whether it worked and how much free space the disk gained over it. The
// editor and `omega steps show` show the latest.
type customRecord struct {
	When    time.Time     `json:"when"`
	Took    time.Duration `json:"took_ns"`
	OK      bool          `json:"ok"`
	Routine string        `json:"routine,omitempty"`
	// Freed is the free space after the step minus before it, in bytes;
	// negative when the step took space, 0 when it was not measured.
	Freed int64 `json:"freed,omitempty"`
}

// customHistory holds the last runs of each step, newest first.
type customHistory map[string][]customRecord

// keepRuns is how many runs of a step are kept.
const keepRuns = 5

func customHistoryPath() string { return filepath.Join(config.StateDir(), "custom-history.json") }

// loadCustomHistory reads the history; a missing or broken file is empty.
func loadCustomHistory() customHistory {
	h := customHistory{}
	if b, err := os.ReadFile(customHistoryPath()); err == nil {
		if json.Unmarshal(b, &h) != nil {
			return customHistory{}
		}
	}
	return h
}

func (h customHistory) runs(id string) []customRecord { return h[id] }

// last is the latest run of a step that worked.
func (h customHistory) last(id string) (customRecord, bool) {
	for _, r := range h[id] {
		if r.OK {
			return r, true
		}
	}
	return customRecord{}, false
}

// lastRun is when `omega custom` last ran a step; ok is false when it never
// did.
func (h customHistory) lastRun() (when time.Time, ok bool) {
	for _, rs := range h {
		for _, r := range rs {
			if r.When.After(when) {
				when, ok = r.When, true
			}
		}
	}
	return when, ok
}

// failedLately lists, in alphabetical order, the steps that failed in any
// of the runs kept.
func (h customHistory) failedLately() []string {
	var out []string
	for id, rs := range h {
		if slices.ContainsFunc(rs, func(r customRecord) bool { return !r.OK }) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

func (h customHistory) add(id string, r customRecord) {
	h[id] = append([]customRecord{r}, h[id]...)
	if len(h[id]) > keepRuns {
		h[id] = h[id][:keepRuns]
	}
}

// save writes the history. It is a convenience: a failure is not worth
// failing the run for, so the caller may ignore it.
func (h customHistory) save() error {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.StateDir(), 0o755); err != nil {
		return err
	}
	tmp := customHistoryPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, customHistoryPath())
}
