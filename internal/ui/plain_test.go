package ui

import (
	"context"
	"testing"
	"time"
)

// A question that is open when the run is cancelled stops waiting: ctrl+c at
// a confirmation, a choice or the password prompt used to hang there.
func TestAnswerStopsWhenTheRunIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	done := make(chan bool, 1)
	go func() {
		_, ok := answer(ctx, func() string { <-never; return "late" })
		done <- ok
	}()
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Error("a cancelled question returned an answer")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the question is still waiting after the run was cancelled")
	}

	// Once cancelled, no new read starts: it would race with the one that
	// was left behind.
	started := false
	if _, ok := answer(ctx, func() string { started = true; return "" }); ok || started {
		t.Errorf("a read started after the run was cancelled (ok %v, started %v)", ok, started)
	}

	// An answer that comes is returned.
	if v, ok := answer(context.Background(), func() string { return "yes" }); !ok || v != "yes" {
		t.Errorf("answer: %q %v", v, ok)
	}
}

// Handing the agent an answer does not block when the agent has stopped
// listening.
func TestReplyDoesNotBlockACancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	nobody := make(chan string) // unbuffered, no reader
	done := make(chan struct{})
	go func() { reply(ctx, nobody, "x"); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reply blocked on a run that was cancelled")
	}
	listening := make(chan string, 1)
	reply(context.Background(), listening, "x")
	if got := <-listening; got != "x" {
		t.Errorf("reply sent %q", got)
	}
}
