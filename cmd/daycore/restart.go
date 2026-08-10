package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
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
// # The listening socket IS inherited — but only when the address did not move
//
// exec can carry an open fd across, which makes the restart seamless: nothing
// is ever refused, because the socket never stops listening. Connections that
// arrive mid-restart queue in the accept backlog and are served by the new
// image.
//
// ⚠️ The reason this needs a condition at all: **a restart exists to pick up new
// configuration, and the listen address is configuration.** Inheriting blindly
// would make a restart after changing PORT appear to work while still serving
// the old address — a restart that quietly ignored the reason it was asked for.
//
// So the parent passes the fd AND the address it belongs to, and **THE CHILD
// DECIDES**. That is not a detail: only the child has read the new
// configuration, so only the child can tell whether the address moved. A parent
// that decided would be comparing the config against itself.
//
//	same address       inherit → no connection is refused at any point
//	different address  close the inherited fd, bind the new one → the ordinary
//	                   few seconds of refusal, which is what changing a listen
//	                   address means
//
// Unix only. Windows has no exec, and carrying a socket across CreateProcess
// needs WSADuplicateSocket plus a handshake with the target pid — real work for
// a platform that has to spawn-and-rebind anyway.
//
// # Order, and the one thing that must not be reordered
// # Order, and the one thing that must not be reordered
//
//	1. preflight        can we even resolve our own executable? Refuse while we
//	                    still have a connection to refuse on.
//	2. respond 200      the console needs the answer before its socket dies.
//	3. graceful stop    Shutdown → StopTicks → ReleaseWorkerLease → WaitBackground,
//	                    main's existing sequence, unchanged.
//	4. keep the socket  dup the listening fd BEFORE the shutdown closes it. A
//	                    dup is a second reference to the same socket, so
//	                    Shutdown closing the original leaves it listening.
//	5. release          close the store. On Unix nothing after exec runs, so a
//	                    deferred Close would never happen and every restart
//	                    would leak a server-side connection until it timed out.
//	6. replace          exec, or spawn-and-return.
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

// startEnv is the environment this process was STARTED with, captured before
// anything read a .env file.
//
// # ⚠️ Without this, a restart cannot pick up an edited .env — at all
//
// godotenv.Load does not merely parse the file: it calls os.Setenv for every
// key that was not already in the environment. So by the time config.Load
// returns, `os.Environ()` contains the .env values as though the operator had
// exported them. Handing THAT to a replacement means the replacement finds them
// already set, godotenv declines to override them, and **the file it was
// supposed to re-read is ignored**.
//
// The symptom is the worst kind: the restart works, the process comes back, and
// the change the operator made is silently not applied. And it defeats the case
// the button exists for — "storage is down, I fixed DB_DSN in .env, restart".
//
// Passing the ORIGINAL environment instead makes the replacement start exactly
// as this process did and read the file for itself. Anything the operator
// really did export is still there; only the values that came from the file are
// left to the file.
//
// This was found by the address-change e2e, which restarted with a new PORT in
// .env and watched the replacement come back on the old one.
var startEnv []string

func captureStartEnv() { startEnv = os.Environ() }

// strippedEnv is the starting environment without our own restart marker.
//
// Appending a second copy would leave two, and which one a child sees is not
// something to rely on across platforms. Restarting twice is ordinary.
func strippedEnv() []string {
	src := startEnv
	if src == nil {
		// A test or a caller that never went through main(). Falling back to the
		// live environment keeps behaviour sane; it just cannot re-read .env.
		src = os.Environ()
	}
	out := make([]string, 0, len(src))
	for _, kv := range src {
		if strings.HasPrefix(kv, restartEnvKey+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// openListener returns the socket to serve on: the inherited one when the
// previous image handed over a socket for this same address, otherwise a fresh
// bind (retried, on the platform that needs it).
func openListener(addr string, logger *slog.Logger) (net.Listener, bool, error) {
	if ln, ok := inheritedListener(addr, logger); ok {
		return ln, true, nil
	}
	_, restarted := restartRequested()
	ln, err := listenWithRetry(addr, restarted, logger)
	return ln, false, err
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

// The two environment variables that carry a listening socket across exec.
//
// Two, not one: the fd alone would let a child inherit a socket bound to an
// address its own configuration no longer asks for. The address travels with it
// so the child can refuse.
const (
	listenFDEnvKey   = "DAYCORE_LISTEN_FD"
	listenAddrEnvKey = "DAYCORE_LISTEN_ADDR"
)

// inheritedListener returns the socket handed over by the previous image, if it
// is for the address this process actually wants.
//
// ⚠️ THE ADDRESS COMPARISON IS THE WHOLE POINT. `want` is computed from THIS
// process's configuration, which is the configuration the restart existed to
// load. If it differs from what the parent was serving, the inherited fd is
// closed and this returns nothing — the caller then binds normally and the
// restart is an ordinary one with a few seconds of refusal, which is what
// changing a listen address means.
//
// A malformed or unusable fd is treated the same way as a mismatch: say so and
// fall back to binding. Refusing to start because a handover went wrong would
// turn a restart into an outage over an optimisation.
func inheritedListener(want string, logger *slog.Logger) (net.Listener, bool) {
	raw := os.Getenv(listenFDEnvKey)
	if raw == "" {
		return nil, false
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd <= 2 {
		// 0/1/2 are stdio; anything there is a bug in the handover, not a socket.
		logger.Warn("ignoring an unusable inherited listener", "fd", raw)
		return nil, false
	}
	from := os.Getenv(listenAddrEnvKey)
	if from != want {
		logger.Info("not reusing the previous listening socket: the address changed",
			"was", from, "now", want)
		// Close it, or this process holds a socket on an address it is not
		// serving — and the next thing to try that address gets EADDRINUSE from
		// a process that has no idea it is holding it.
		_ = os.NewFile(uintptr(fd), "old-listener").Close()
		return nil, false
	}
	ln, err := net.FileListener(os.NewFile(uintptr(fd), "inherited-listener"))
	if err != nil {
		logger.Warn("could not adopt the inherited listening socket; binding instead", "err", err)
		return nil, false
	}
	logger.Info("reusing the previous listening socket — no connection was refused during this restart",
		"addr", want)
	return ln, true
}

// socketHandover is a listening socket prepared to survive the replacement.
//
// ⚠️ IT MUST BE PREPARED BEFORE THE GRACEFUL SHUTDOWN. Shutdown closes the
// listener, and a dup taken afterwards fails with "use of closed network
// connection" — which degrades silently to a rebind, because the handover is
// best-effort by design. The restart still works; it just stops being seamless,
// and nothing says so.
//
// That failure is not hypothetical: this type exists because the first version
// prepared the handover inside replaceSelf, which runs after the shutdown, and
// every restart quietly fell back. The e2e caught it.
type socketHandover struct {
	fd   int
	addr string
	ok   bool
}

// env returns the variables that carry the handover across, or nothing.
func (h socketHandover) env() []string {
	if !h.ok {
		return nil
	}
	return []string{
		listenFDEnvKey + "=" + strconv.Itoa(h.fd),
		listenAddrEnvKey + "=" + h.addr,
	}
}
