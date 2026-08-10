//go:build !windows

package main

import (
	"log/slog"
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
func replaceSelf(logger *slog.Logger, exe string) error {
	// The marker the child reads back. It arms the bind retry (which this path
	// will not need) and, more usefully, puts "this is a restart, from pid N"
	// into the startup log where somebody debugging will see it.
	env := append(strippedEnv(), restartEnvKey+"="+strconv.Itoa(os.Getpid()))

	// argv[0] must be included: execve takes the whole argv, unlike
	// exec.Command which prepends the path for you. Omitting it shifts every
	// flag by one, which would silently feed the first flag's value to a
	// program that thinks it is the program name.
	argv := append([]string{exe}, os.Args[1:]...)

	logger.Info("replacing this process image", "exe", exe, "pid", os.Getpid())
	// Returns only on failure.
	return syscall.Exec(exe, argv, env)
}
