package server

import (
	"context"
	"runtime/debug"
	"time"
)

// everyTick runs fn on an interval, on its own goroutine, and survives a panic
// inside fn.
//
// The two cleanup loops used to be bare `go func() { for range ticker.C { … } }`
// with no recover. That is fine while the store is healthy and fatal the moment
// it is not: `recoverMW` only wraps HTTP handlers, so a panic on a free goroutine
// takes the whole process down. With the planned degraded boot (serving the
// console while storage is unavailable) the store is a stub that never panics —
// but a nil one would have produced the worst possible symptom: the server starts
// fine, answers requests, and dies silently sixty seconds later with a stack in
// the logs that mentions a ticker nobody was thinking about.
//
// A panic kills that one tick, not the loop. A cleanup pass that fails is a few
// stale rows; a cleanup loop that stops is unbounded growth, and stopping it is
// the more expensive choice.
// # It must be stoppable, and it was not
//
// The original loop had no stop signal at all — no context, no channel, no
// return value. That was defensible while every tick was stateless cleanup
// ("进程退出即弃，无碍"), and it stops being defensible the moment a tick owns
// something: a lease-renewal tick that keeps running through shutdown will
// re-acquire the lease its own process just released, and the next instance
// waits a full TTL for a leader that has already exited.
//
// StopTicks is called on the shutdown path, before the store closes. A tick
// already in flight finishes; no new one starts.
func (s *Server) everyTick(name string, interval time.Duration, fn func(context.Context)) {
	s.tickOnce.Do(func() { s.ticksDone = make(chan struct{}) })
	done := s.ticksDone
	s.tickWG.Add(1)
	go func() {
		defer s.tickWG.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				s.runTick(name, fn)
			}
		}
	}()
}

// everyTickNow is everyTick with one immediate run before the first interval.
//
// time.Ticker's first tick lands one whole interval in, which is right for
// cleanup (a fresh process has nothing stale yet) and wrong for anything that
// establishes state — a lease loop that waits 20 seconds leaves the deployment
// with no leader for those 20 seconds after every restart.
func (s *Server) everyTickNow(name string, interval time.Duration, fn func(context.Context)) {
	s.runTick(name, fn)
	s.everyTick(name, interval, fn)
}

// StopTicks ends every background tick loop and waits for an in-flight tick.
// Idempotent: shutdown paths get called twice more often than anyone expects.
func (s *Server) StopTicks() {
	s.tickOnce.Do(func() { s.ticksDone = make(chan struct{}) })
	s.tickStop.Do(func() { close(s.ticksDone) })
	s.tickWG.Wait()
}

// runTick is separate so the deferred recover scopes to one iteration.
func (s *Server) runTick(name string, fn func(context.Context)) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("background tick panicked; the loop continues",
				"tick", name, "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	fn(context.Background())
}
