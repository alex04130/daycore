package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Restarting the process from the console, on both platforms, with one code
// path.
//
// # Two candidates, and why this is the one
//
//	exec self          syscall.Exec on Unix. Same pid, listening fd inherited,
//	                   nothing to coordinate. And it DOES NOT EXIST ON WINDOWS,
//	                   so that platform needs the other one anyway — leaving two
//	                   implementations, two failure modes, and one of them only
//	                   ever exercised by whoever happens to run it.
//	spawn then exit    identical on both platforms. The cost is a window where
//	                   the old process still holds the port and the new one
//	                   wants it.
//
// The second, because "consistent across Windows and Linux" was the
// requirement, and one path that is the same everywhere beats two paths that
// are each simpler.
//
// # The port race is closed by RETRYING, not by socket options
//
// The obvious fix is SO_REUSEADDR, and it is the wrong one here: its semantics
// differ between Unix and Windows (on Windows it lets a second socket STEAL a
// live listener), which is exactly the platform-specific behaviour this design
// exists to avoid.
//
// So the child simply retries its bind for a few seconds. The parent shuts down
// gracefully — which closes the listener — and the child's next attempt
// succeeds. No shared state, no signalling, no platform difference.
//
// ⚠️ The retry is ONLY armed for a restart (the parent sets DAYCORE_RESTART_FROM).
// A normal boot must still fail immediately when the port belongs to something
// else: "address already in use" after ten silent seconds is a much worse
// answer than the same message straight away.
//
// # Order, and the one thing that must not be reordered
//
//	1. preflight        can we even resolve our own executable? Refuse while we
//	                    still have a connection to refuse on.
//	2. respond 200      the console needs the answer before its socket dies.
//	3. graceful stop    Shutdown → StopTicks → ReleaseWorkerLease → WaitBackground,
//	                    main's existing sequence, unchanged.
//	4. spawn            after the listener is closed, so the child's first
//	                    attempt usually succeeds.
//	5. exit             and if the spawn failed, DO NOT exit — see below.
//
// ⚠️ If the spawn fails at step 4 the process must NOT exit. It has already
// stopped serving, so it is no use as it is — but a process that is up and
// broken can still be inspected, and its logs are still attached to the
// terminal that started it. Exiting turns a visible failure into a machine that
// stopped answering for no stated reason.

// restartEnvKey tells a child process that it was spawned by a restart, which
// arms the bind retry and gets the fact into the startup log.
const restartEnvKey = "DAYCORE_RESTART_FROM"

// restartBindWait is how long a restarted child keeps trying to bind.
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

// spawnSelf starts a fresh copy of this program and returns once it has been
// launched — NOT once it is serving.
//
// Deliberately no SysProcAttr, which is the other place a platform difference
// would creep in. The child inherits stdin/stdout/stderr, so its logs land
// wherever the parent's did, and it survives the parent's exit on both
// platforms without being detached — a server does not need its own session,
// and asking for one would mean a build-tagged file per platform.
func spawnSelf(logger *slog.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...) //nolint:gosec // our own path, our own args
	cmd.Env = append(strippedEnv(), restartEnvKey+"="+strconv.Itoa(os.Getpid()))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	logger.Info("started the replacement process", "pid", cmd.Process.Pid, "exe", exe)
	// Released rather than waited on: this process is about to exit, and Wait
	// would block for the child's whole lifetime. Release also stops the runtime
	// from reaping it on our way out.
	return cmd.Process.Release()
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
