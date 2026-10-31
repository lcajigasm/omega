package llm

import "time"

// retryDelay is the pause before a repeated attempt. attempt is the loop
// index, so the first retry waits 2s, then 4s, then 8s. The Anthropic client
// and the OpenAI-compatible client share this schedule.
func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<attempt) * time.Second
}
