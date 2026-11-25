package agent

import (
	"fmt"
	"strings"
	"sync"
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

// ResultCache keeps the read-only results of one run. A session makes its
// own; the stages of a guided or unattended run share one, so the disk scan
// and the dry runs the first stage measured are not measured again by every
// stage after it. Anything that may change the machine, in any stage,
// clears it for all of them.
type ResultCache struct {
	mu      sync.Mutex
	results map[string]cachedResult
}

// NewResultCache is an empty cache for the stages of one run.
func NewResultCache() *ResultCache { return &ResultCache{} }

func (c *ResultCache) get(key string) (cachedResult, bool) {
	if c == nil {
		return cachedResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.results[key]
	if ok && time.Since(r.at) > cacheFor {
		delete(c.results, key)
		return cachedResult{}, false
	}
	return r, ok
}

func (c *ResultCache) put(key, out string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.results == nil {
		c.results = map[string]cachedResult{}
	}
	c.results[key] = cachedResult{out: out, at: time.Now()}
}

// Clear drops everything: something outside the sessions (a hook, a step
// run without the model) may have changed the machine.
func (c *ResultCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.results = nil
	c.mu.Unlock()
}

// Len is how many results the cache keeps.
func (c *ResultCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.results)
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
	c, ok := a.cache.get(tool + "\x00" + command)
	if !ok {
		return "", false
	}
	mins := int(time.Since(c.at).Minutes())
	a.writeLog(Event{Kind: EvAssistant, Text: fmt.Sprintf("[%s: result from %d min ago in this run, not run again]", tool, mins)})
	return fmt.Sprintf("(same result as %d minutes ago in this run; nothing has changed since, so it was not run again)\n%s", mins, c.out), true
}

func (a *Agent) remember(tool, command, out string) {
	if !cacheable(tool) {
		return
	}
	a.cache.put(tool+"\x00"+command, out)
}

func (a *Agent) forget() { a.cache.Clear() }
