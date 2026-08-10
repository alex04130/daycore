//go:build !windows

package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"syscall"
)

// replaceSelf replaces this process image with a fresh one. It DOES NOT RETURN
// on success — there is no "after" to return to.
//
// # Why exec is the right answer everywhere it exists
//
// Linux, macOS and the BSDs all have execve, and it is strictly better than
// spawning a child:
//
//   - **No coexistence window.** The old process ceases to exist at the instant
//     the new image starts, so there is never a moment when two of them want the
//     same port. The Windows path has to retry its bind; this one does not.
//   - **No orphan.** Nothing to reparent, nothing to leak if the parent dies
//     between spawning and exiting.
//   - **The pid does not move.** Anything watching this process — a shell job, a
//     `systemd` unit with `Type=exec`, a container's pid 1 — keeps watching the
//     right thing. A spawn-and-exit restart makes pid 1 exit, which in a
//     container means the container stops.
//
// That last point is the one that would have bitten hardest: `deploy/` runs this
// binary as the container's main process.
//
// # Failure is a plain error, which is the other reason
//
// execve only replaces the image on SUCCESS. If it fails — the file vanished,
// the mount went read-only, the architecture is wrong — this function returns
// and the process is still alive to say so. The spawn-based path has to reason
// about "we already shut down and the child did not start"; here that state
// simply is not reachable.
//
// ⚠️ NOTHING AFTER THIS RUNS. No deferred function, no atexit, no buffered
// writer flush. The caller must have released everything it owns first — see
// finishRestart, which closes the store before calling this and says why.
func replaceSelf(logger *slog.Logger, exe string, h socketHandover) error {
	// The marker the child reads back. It arms the bind retry (which this path
	// will not need) and, more usefully, puts "this is a restart, from pid N"
	// into the startup log where somebody debugging will see it.
	env := append(strippedEnv(), restartEnvKey+"="+strconv.Itoa(os.Getpid()))
	env = append(env, h.env()...)

	// argv[0] must be included: execve takes the whole argv, unlike
	// exec.Command which prepends the path for you. Omitting it shifts every
	// flag by one, which would silently feed the first flag's value to a
	// program that thinks it is the program name.
	argv := append([]string{exe}, os.Args[1:]...)

	logger.Info("replacing this process image", "exe", exe, "pid", os.Getpid(), "handedOverSocket", h.ok)
	// Returns only on failure.
	return syscall.Exec(exe, argv, env)
}

// prepareHandover dups the listening socket so it survives the replacement.
//
// ⚠️ CALL IT BEFORE THE GRACEFUL SHUTDOWN. See socketHandover for what happens
// when it is called after — the answer is "nothing visible", which is why the
// ordering is a named step in main's restart path rather than a detail here.
//
// Two things have to be true and neither is the default:
//
//	a DUP        (*net.TCPListener).File() returns one, which is what lets the
//	             socket outlive the Shutdown that closes the listener: a dup is
//	             a second reference to the same socket, so the socket stays in
//	             LISTEN state and arriving connections queue instead of being
//	             refused.
//	no CLOEXEC   Go sets FD_CLOEXEC on every descriptor it opens, so without
//	             clearing it execve closes the fd and the child inherits a
//	             number pointing at nothing.
//
// Best-effort throughout: any failure means the replacement binds for itself,
// which is what happened before this existed. An outage would be a bad price
// for a failed optimisation.
func prepareHandover(ln net.Listener, addr string, logger *slog.Logger) socketHandover {
	tl, ok := ln.(*net.TCPListener)
	if !ok {
		logger.Warn("cannot hand over the listening socket", "type", fmt.Sprintf("%T", ln))
		return socketHandover{}
	}
	f, err := tl.File()
	if err != nil {
		logger.Warn("cannot hand over the listening socket; the replacement will bind for itself", "err", err)
		return socketHandover{}
	}
	fd := int(f.Fd())
	// F_SETFD with 0 clears every descriptor flag, and FD_CLOEXEC is the only
	// one defined — so this is "keep it across exec" rather than a blunt reset.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, 0); errno != 0 {
		f.Close()
		logger.Warn("cannot clear close-on-exec on the listening socket", "err", errno)
		return socketHandover{}
	}
	// ⚠️ f is deliberately NOT closed and NOT allowed to be finalised: closing it
	// would close the very descriptor the child is about to be told to use.
	keepAlive = append(keepAlive, f)
	logger.Info("prepared the listening socket for handover", "fd", fd, "addr", addr)
	return socketHandover{fd: fd, addr: addr, ok: true}
}

// keepAlive holds the handed-over file so its finaliser cannot close the very
// descriptor the child was told about. The process is about to be replaced, so
// this never grows.
var keepAlive []*os.File
