//go:build windows

package main

// parentExitsOnRestart is this platform's expected outcome.
//
// Windows has no exec, so the restart spawns a replacement and the original
// exits — which is what frees the port for it. Stated as a constant beside the
// implementation that makes it true (restart_windows.go) rather than as a
// runtime.GOOS check buried in the test body.
const parentExitsOnRestart = true
