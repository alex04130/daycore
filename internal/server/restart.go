package server

import (
	"net/http"
	"sync/atomic"

	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (restart)", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/admin/restart", s.handleAdminRestart)
	})
}

// Restarting the process from the console.
//
// # It really restarts. It does not exit and hope.
//
// The tempting implementation is `os.Exit(0)` and let a supervisor bring the
// process back. That works under systemd or Docker and turns the button into
// **"switch the server off, permanently"** for anybody running the binary
// directly — which is the deployment shape `daycore install` produces and the
// one this project tells people is supported. It would be the most expensive
// mistake this console is capable of, so the process brings itself back.
//
// # Why the process package does not live here
//
// This file decides WHO may restart and WHEN the answer goes out. Actually
// replacing the process is cmd/daycore's job, installed with SetRestarter,
// because the shutdown sequence it has to run first (Shutdown → StopTicks →
// ReleaseWorkerLease → WaitBackground) is main's, in main's order, and a second
// copy of that order living in this package is a second copy that will drift.
//
// The seam also makes the interesting half testable without a process: a test
// can install a restarter that records the call.
//
// # ⚠️ Degraded mode is exactly when this matters
//
// Storage is down, the operator has fixed DB_DSN in .env, and the ONLY way to
// pick it up is a restart. So this must not need the store — it does not, and
// the authorisation path it uses was already built to work with no database.
// In a degraded process only the root credential passes anything, which is the
// right answer here rather than a limitation.

var (
	keyAdminRestartUnavailable = i18n.Reg("admin.restart.unavailable", i18n.Text{
		"zh-CN": "这个进程没法自己重启：",
		"en-US": "This process cannot restart itself: ",
	})
	keyAdminRestartNoHook = i18n.Reg("admin.restart.no_hook", i18n.Text{
		"zh-CN": "这个版本没有装重启入口 —— 它是由 cmd/daycore 安装的，所以只有真正的服务进程才有",
		"en-US": "This build has no restart hook — it is installed by cmd/daycore, so only a real server process has one",
	})
	keyAdminRestartAccepted = i18n.Reg("admin.restart.accepted", i18n.Text{
		"zh-CN": "正在重启。在途的请求会先收尾，然后进程把自己换掉 —— 几秒内连不上是正常的。如果一分钟后还连不上，就要有人登机器看日志了。",
		"en-US": "Restarting. In-flight requests finish first, then the process replaces itself — a few seconds of refused connections is normal. If it is still refusing after a minute, somebody needs to look at the logs on the machine.",
	})
)

// SetRestarter installs the process-level restart, which cmd/daycore owns.
//
// The function must PREFLIGHT synchronously and return an error if the restart
// cannot be started — the console needs that answer while it still has a
// connection to be told on. Everything after the preflight happens once this
// handler has returned, because graceful shutdown waits for handlers and a
// handler waiting for shutdown would wait for itself.
func (s *Server) SetRestarter(fn func() error) {
	s.restarter.Store(&fn)
}

// SetListenerInherited records whether this process adopted its predecessor's
// listening socket.
//
// It exists so the console can tell the operator which kind of restart the last
// one was — seamless, or a rebind with a gap. That distinction is not cosmetic:
// "did anybody get a connection refused" is the first question after a restart,
// and the answer depends on whether the listen address changed, which is
// something the operator may have done in .env without connecting the two
// facts.
func (s *Server) SetListenerInherited(v bool) { s.listenerInherited.Store(v) }

// ListenerInherited reports what SetListenerInherited was told.
func (s *Server) ListenerInherited() bool { return s.listenerInherited.Load() }

// CanRestart reports whether a restarter is installed at all.
//
// Nothing but the console reads this today; it exists so the console can grey
// out a button rather than offer one that answers 501 — the same rule the
// database screen follows for a table it cannot delete from.
func (s *Server) CanRestart() bool { return s.restarter.Load() != nil }

// POST /api/admin/restart
func (s *Server) handleAdminRestart(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	fn := s.restarter.Load()
	if fn == nil {
		// A build or a test with no process behind it. 501 rather than 500: the
		// request was fine, this deployment just has nothing to restart.
		s.writeErr(w, http.StatusNotImplemented, "restart_unavailable", i18n.T(keyAdminRestartNoHook, locale))
		return
	}
	if err := (*fn)(); err != nil {
		// The preflight failed — most likely the executable path could not be
		// resolved. Refuse LOUDLY and stay up: the alternative is shutting down
		// and then discovering there is nothing to start.
		s.log.Error("restart refused during preflight", "err", err)
		s.writeErr(w, http.StatusServiceUnavailable, "restart_unavailable",
			i18n.T(keyAdminRestartUnavailable, locale)+err.Error())
		return
	}
	s.log.Warn("restart requested from the console", "root", s.isRootCredential(r))
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": i18n.T(keyAdminRestartAccepted, locale),
	})
}

// restarterHolder is the field type; kept as its own name so the zero value of
// Server stays usable (RouteTable builds one).
type restarterHolder = atomic.Pointer[func() error]
