package llm

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter caps a server-supplied pause. One 429 must not stall a run
// for the minutes some APIs ask for.
const maxRetryAfter = 30 * time.Second

// nowFn is the clock retryWait reads. Tests pin it.
var nowFn = time.Now

// retryDelay is the pause before a repeated attempt when the server gave no
// Retry-After. attempt is the loop index, so the first retry waits 2s, then
// 4s, then 8s. The Anthropic client and the OpenAI-compatible client share it.
func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * time.Second
}

// retryWait is how long to wait before attempt. A Retry-After delay in
// seconds, or an HTTP date, replaces the schedule. The wait is never longer
// than maxRetryAfter. An empty or unreadable header keeps the schedule.
func retryWait(attempt int, retryAfter string) time.Duration {
	if d, ok := parseRetryAfter(retryAfter, nowFn()); ok {
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	return retryDelay(attempt)
}

func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n < 0 {
			return 0, false
		}
		return time.Duration(n) * time.Second, true
	}
	when, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := when.Sub(now)
	if d < 0 {
		return 0, true
	}
	return d, true
}
