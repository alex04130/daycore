//go:build windows

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// replaceSelf starts a fresh copy of this program and RETURNS. The caller then
// exits normally, which is what lets the replacement have the port.
//
// # Windows has no exec, so this is the only shape available
//
// Not a preference. CreateProcess makes a new process; there is no call that
// replaces the current image the way execve does. Everything below is the
// consequence of that.
//
// # What the caller must do differently from the Unix path
//
//	Unix     replaceSelf never returns; nothing after it runs.
//	Windows  replaceSelf returns nil and the caller must let run() return, so
//	         main exits with status 0 and releases the port.
//
// The two contracts are stated here rather than hidden behind a shared name
// that quietly means two things. finishRestart is written against both.
//
// # No SysProcAttr, on purpose
//
// The child inherits stdin/stdout/stderr, so its logs land wherever the
// parent's did — which is the whole reason an operator can see what happened.
// It survives the parent's exit without being detached: this process creates no
// job object, so nothing ties the child's lifetime to it.
//
// ⚠️ CREATE_NEW_PROCESS_GROUP is deliberately NOT set. It would stop Ctrl-C in
// the console window from reaching the new process — turning a restart into a
// server the operator can no longer stop the way they started it.
//
// # The port
//
// Both processes exist for a moment, and Go's net.Listen does not set
// SO_REUSEADDR on Windows (deliberately — there it lets a second socket steal a
// live listener). So the child retries its bind; see listenWithRetry, armed by
// the marker set below.
func replaceSelf(logger *slog.Logger, exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...) //nolint:gosec // our own path, our own args
	cmd.Env = append(strippedEnv(), restartEnvKey+"="+strconv.Itoa(os.Getpid()))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	logger.Info("started the replacement process", "pid", cmd.Process.Pid, "exe", exe)
	// Released rather than waited on: this process is about to exit, and Wait
	// would block for the child's whole lifetime.
	return cmd.Process.Release()
}
