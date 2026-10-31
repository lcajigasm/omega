package llm

import (
	"testing"
	"time"
)

func TestRetryDelayMatchesOpenAISchedule(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt, d := range want {
		if got := retryDelay(attempt); got != d {
			t.Errorf("retryDelay(%d) = %s, want %s", attempt, got, d)
		}
	}
}
