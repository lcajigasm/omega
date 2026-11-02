package llm

import (
	"net/http"
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

func TestRetryAfterReplacesTheScheduleAndIsCapped(t *testing.T) {
	now := time.Date(2026, 11, 2, 6, 0, 0, 0, time.UTC)
	prev := nowFn
	nowFn = func() time.Time { return now }
	t.Cleanup(func() { nowFn = prev })

	cases := []struct {
		header string
		want   time.Duration
	}{
		{"12", 12 * time.Second},
		{"120", maxRetryAfter},
		{"0", 0},
		{"", 2 * time.Second},
		{"soon", 2 * time.Second},
		{now.Add(5 * time.Second).Format(http.TimeFormat), 5 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}
	for _, c := range cases {
		if got := retryWait(1, c.header); got != c.want {
			t.Errorf("retryWait(%q) = %s, want %s", c.header, got, c.want)
		}
	}
}
