//go:build !windows

package main

// parentExitsOnRestart is this platform's expected outcome.
//
// Unix restarts with syscall.Exec, which REPLACES the process image: the
// original process never exits, so a Wait on it never returns. That is the
// property worth pinning — an implementation that quietly switched to
// spawn-and-exit would still change the instance id and still pass every other
// assertion, while losing the three things exec was chosen for: no coexistence
// window, no orphan, and pid 1 in a container staying alive.
const parentExitsOnRestart = false
