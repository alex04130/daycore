package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"
)

// Restarting the process from the console.
//
// # Each platform gets the mechanism that platform actually has
//
//	Unix (Linux, macOS, BSD)   syscall.Exec — the process image is REPLACED.
//	                           Same pid, no moment when two of them exist, and
//	                           on failure exec simply returns, so the "spawn
//	                           failed, now what" case is a plain error rather
//	                           than a hole.
//	Windows                    spawn then exit. There is no exec on Windows;
//	                           this is not a preference.
//
// The first draft used spawn-then-exit everywhere, on the theory that one path
// beats two because the second path is only exercised by whoever happens to run
// that platform. ⚠️ That reasoning was wrong here, and the author said so:
// **a cross-platform binary is cross-compiled anyway**, so each path is built
// and can be exercised as part of the same release step. The cost of a second
// path is therefore a build target, not an untested branch — and in exchange
// Unix gets the mechanism with no coexistence window at all.
//
// `GOOS=windows go vet ./...` type-checks the branch this machine never runs,
// and it is in the local verification list in CLAUDE.md for exactly that
// reason: a build-tagged file nobody compiles is a file that stops compiling.
//
// # The port handover, and why only one platform needs help with it
//
// After exec there is no old process, so there is nothing to hand over. After a
// Windows spawn both exist for a moment, and the child would lose the bind.
//
// It is closed by the child RETRYING its bind, not by SO_REUSEADDR — whose
// semantics differ between the two platforms in the direction that matters (on
// Windows it lets a second socket steal a LIVE listener). Go's net.Listen
// already sets it on Unix and deliberately does not on Windows, which is
// exactly the split this reasoning follows.
//
// ⚠️ The retry is only armed for a restart (the parent sets
// DAYCORE_RESTART_FROM). A normal boot must still fail immediately when the
// port belongs to something else: "address already in use" after thirty silent
// seconds is a much worse answer than the same message straight away.
//
// # ⚠️ The listening socket is deliberately NOT inherited
//
// exec could pass the fd through and make the restart zero-downtime. It must
// not: a restart exists to pick up NEW CONFIGURATION, and the listen address is
// configuration. Inheriting the old socket would silently keep serving the old
// address after somebody changed PORT — a restart that appears to work and
// quietly ignored the reason it was asked for.
//
// # Order, and the one thing that must not be reordered
//
//	1. preflight        can we even resolve our own executable? Refuse while we
//	                    still have a connection to refuse on.
//	2. respond 200      the console needs the answer before its socket dies.
//	3. graceful stop    Shutdown → StopTicks → ReleaseWorkerLease → WaitBackground,
//	                    main's existing sequence, unchanged.
//	4. release          close the store. On Unix nothing after exec runs, so a
//	                    deferred Close would never happen and every restart
//	                    would leak a server-side connection until it timed out.
//	5. replace          exec, or spawn-and-return.
//
// ⚠️ If step 5 fails the process must NOT exit. It has already stopped serving,
// so it is no use as it is — but a process that is up and broken can still be
// inspected, and its logs are still attached to whatever started it. Exiting
// turns a visible failure into a machine that stopped answering for no stated
// reason.

// restartEnvKey tells a child process that it was spawned by a restart, which
// arms the bind retry and gets the fact into the startup log.
const restartEnvKey = "DAYCORE_RESTART_FROM"

// restartBindWait is how long a restarted child keeps trying to bind.
//
// ⚠️ Only Windows ever needs it — on Unix the old process is already gone by
// the time the new image runs. It is armed on both because the marker is set on
// both, and a retry that never fires costs nothing; making it conditional would
// mean one more platform difference to keep straight.
//
// Generous on purpose. The parent's graceful shutdown waits for in-flight
// requests and for detached agent turns, with a 15-second deadline of its own,
// so the child can legitimately be waiting for most of that. A retry window
// shorter than the parent's shutdown deadline would turn a slow drain into a
// failed restart.
const (
	restartBindWait  = 30 * time.Second
	restartBindEvery = 200 * time.Millisecond
)

// restartRequested reports whether this process was spawned by a restart.
func restartRequested() (fromPID string, yes bool) {
	v := os.Getenv(restartEnvKey)
	return v, v != ""
}

// preflightRestart answers "could this process replace itself", without doing
// it.
//
// Called synchronously from the HTTP handler so a refusal reaches the operator
// on the connection they asked from. The one thing that can genuinely be
// missing is our own path: os.Executable fails on a few exotic setups, and
// finding that out AFTER shutting down would be the worst possible moment.
func preflightRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve this program's own path: %w", err)
	}
	if _, err := os.Stat(exe); err != nil {
		// The binary was deleted or replaced under a running process. On Linux
		// os.Executable resolves /proc/self/exe, which still points at the
		// deleted inode — so this catches the case where a deploy removed the
		// file and a restart would have nothing to start.
		return fmt.Errorf("this program's file is gone (%s): %w", exe, err)
	}
	return nil
}

// strippedEnv is the environment without our own restart marker.
//
// Appending a second copy would leave two, and which one a child sees is not
// something to rely on across platforms. Restarting twice is ordinary.
func strippedEnv() []string {
	src := os.Environ()
	out := make([]string, 0, len(src))
	for _, kv := range src {
		if strings.HasPrefix(kv, restartEnvKey+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// listenWithRetry opens the listening socket, retrying only for a restart.
//
// Returning the listener rather than calling ListenAndServe is the whole point:
// the bind has to be a step this code controls, and http.Server.ListenAndServe
// does it internally with no way in.
func listenWithRetry(addr string, restarted bool, logger *slog.Logger) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil || !restarted {
		return ln, err
	}
	// Only here: we were spawned by a restart, so the address being busy is
	// almost certainly the parent still draining. Say so, once, rather than
	// looking hung.
	logger.Info("waiting for the previous process to let go of the address",
		"addr", addr, "for", restartBindWait)
	deadline := time.Now().Add(restartBindWait)
	for time.Now().Before(deadline) {
		time.Sleep(restartBindEvery)
		if ln, err = net.Listen("tcp", addr); err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("the address was still in use %s after a restart: %w", restartBindWait, err)
}

// errRestartNotPossible is what the console is told when the preflight fails.
var errRestartNotPossible = errors.New("this process cannot restart itself")
