package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/safety"
)

// The session's cache of read-only results.

// Results of read-only measurements are kept for the session: runs went
// through the same 70-second disk scan three times, and the same dry run
// before every decision. Anything that may change the machine clears them.
const cacheFor = 15 * time.Minute

type cachedResult struct {
	out string
	at  time.Time
}

// cacheable are the read-only tools whose results are kept. run_command is
// not: a command may be repeated on purpose to see a change.
func cacheable(tool string) bool {
	return tool == "disk_usage" || strings.HasPrefix(tool, "run_step:")
}

func (a *Agent) cached(tool, command string, risk safety.Risk) (string, bool) {
	if risk != safety.Safe || !cacheable(tool) {
		return "", false
	}
	a.mu.Lock()
	c, ok := a.results[tool+"\x00"+command]
	a.mu.Unlock()
	if !ok || time.Since(c.at) > cacheFor {
		return "", false
	}
	mins := int(time.Since(c.at).Minutes())
	a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[%s: result from %d min ago in this session, not run again]", tool, mins)})
	return fmt.Sprintf("(same result as %d minutes ago in this session; nothing has changed since, so it was not run again)\n%s", mins, c.out), true
}

func (a *Agent) remember(tool, command, out string) {
	if !cacheable(tool) {
		return
	}
	a.mu.Lock()
	if a.results == nil {
		a.results = map[string]cachedResult{}
	}
	a.results[tool+"\x00"+command] = cachedResult{out: out, at: time.Now()}
	a.mu.Unlock()
}

func (a *Agent) forget() {
	a.mu.Lock()
	a.results = nil
	a.mu.Unlock()
}
