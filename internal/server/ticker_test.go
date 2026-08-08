package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// everyTick had no stop signal at all — no context, no channel, no return value.
//
// That was defensible while every tick was stateless cleanup, and it stops being
// defensible the moment a tick owns something: a lease-renewal loop that keeps
// running through shutdown re-acquires the lease its own process just released,
// and the next instance waits a full TTL for a leader that has already exited.
// Batch ζ is about to add exactly that loop.
func TestStopTicksEndsTheLoop(t *testing.T) {
	s := &Server{log: discardLogger()}
	var runs atomic.Int64
	s.everyTick("test", time.Millisecond, func(context.Context) { runs.Add(1) })

	waitFor(t, func() bool { return runs.Load() > 0 }, "the tick never ran")
	s.StopTicks()
	after := runs.Load()

	time.Sleep(20 * time.Millisecond) // many intervals
	if got := runs.Load(); got != after {
		t.Errorf("the loop ran %d more times after StopTicks", got-after)
	}
	// Shutdown paths get called twice more often than anyone expects.
	s.StopTicks()
}

// StopTicks must wait for a tick that is already running, not just stop
// scheduling new ones: the point is that nothing re-enters the store after the
// shutdown path decides it is done.
func TestStopTicksWaitsForATickInFlight(t *testing.T) {
	s := &Server{log: discardLogger()}
	started := make(chan struct{})
	release := make(chan struct{})
	var finished atomic.Bool
	s.everyTick("slow", time.Millisecond, func(context.Context) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		finished.Store(true)
	})

	<-started
	go func() { time.Sleep(10 * time.Millisecond); close(release) }()
	s.StopTicks()
	if !finished.Load() {
		t.Error("StopTicks returned while a tick was still running")
	}
}

// A ticker's first tick lands one whole interval in. That is right for cleanup
// (a fresh process has nothing stale yet) and wrong for anything that
// establishes state — a lease loop that waits its whole interval leaves the
// deployment with no leader for that long after every restart.
func TestEveryTickNowRunsImmediately(t *testing.T) {
	s := &Server{log: discardLogger()}
	var runs atomic.Int64
	s.everyTickNow("boot", time.Hour, func(context.Context) { runs.Add(1) })
	if got := runs.Load(); got != 1 {
		t.Errorf("ran %d times before the first interval, want 1", got)
	}
	s.StopTicks()
}

// A panic kills one tick, not the loop: a cleanup pass that fails is a few stale
// rows, a cleanup loop that stops is unbounded growth.
func TestTickPanicDoesNotKillTheLoop(t *testing.T) {
	s := &Server{log: discardLogger()}
	var runs atomic.Int64
	s.everyTick("panicky", time.Millisecond, func(context.Context) {
		if runs.Add(1) == 1 {
			panic("first tick")
		}
	})
	waitFor(t, func() bool { return runs.Load() >= 3 }, "the loop stopped after a panic")
	s.StopTicks()
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal(msg)
}
