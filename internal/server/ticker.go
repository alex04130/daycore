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
func (s *Server) everyTick(name string, interval time.Duration, fn func(context.Context)) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			s.runTick(name, fn)
		}
	}()
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
