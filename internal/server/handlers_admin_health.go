package server

import (
	"net/http"
	"time"

	"daycore/internal/version"
)

func init() {
	registerRoutes("admin (health)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/health", s.handleAdminHealth)
	})
}

// startedAt is when this process began serving.
//
// A package-level variable set at init rather than a field on Server, because
// the question it answers is about the PROCESS: "did this just restart?" is the
// first thing an operator asks when something is wrong, and a Server built
// twice in one process (which tests do) should not report two different
// answers.
var startedAt = time.Now()

// GET /api/admin/health — everything the overview screen needs, in one call.
//
// # Why this exists when /api/healthz already does
//
// Because /api/healthz is unauthenticated and therefore cannot say anything
// useful. It reports that storage is unavailable and deliberately refuses to
// say why: a driver error routinely carries the DSN, and a DSN routinely
// carries a password. Its comment says "the operator gets the reason through
// the console" — and until this endpoint, **that was not true of any code**.
// DegradedReason() existed, was documented as the console's path, and had
// exactly one caller: a log line in main. The console could not reach it.
//
// # What is here that is nowhere else
//
//   - **reason** — the actual driver error. The single most useful string in
//     the deployment when storage is down, and the one thing healthz must not
//     say.
//   - **uptime** — "did this just restart" is how an operator tells a
//     configuration problem from a crash loop, and nothing in this codebase
//     recorded a start time before now.
//   - **instance** — health is per process by design (see internal/adapters),
//     so two consoles behind a load balancer legitimately disagree. Without a
//     name attached, that is an unexplainable bug report rather than a fact.
//
// # Boundary: admin-only, and that is the whole reason it can be honest
//
// Everything above is either an internal detail or a timing signal. The
// unauthenticated endpoint stays exactly as terse as it is.
func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminHealth.unauthorized")
		return
	}

	out := map[string]any{
		"degraded":  s.Degraded(),
		"instance":  s.InstanceID(),
		"startedAt": startedAt.UTC(),
		"uptimeSec": int64(time.Since(startedAt).Seconds()),
		"db":        s.cfg.DBType,
		"env":       s.cfg.Env,
		"build":     version.Full(),
		"channel":   version.Channel,
		// Whether this build carries a console at all. An operator reading this
		// through curl is entitled to know before going looking for /admin.
		"console": ConsoleBuilt(),
	}

	if s.Degraded() {
		// ⚠️ The driver error, and only for the root credential.
		//
		// This endpoint shipped an hour before this line was written, with the
		// reason available to anyone holding any admin credential. That was
		// wrong for a reason the endpoint's own doc comment states two
		// paragraphs earlier and then walks past: **a driver error routinely
		// carries the DSN, and a DSN routinely carries a password.** Withholding
		// it from unauthenticated callers and handing it to every console user
		// gets the boundary exactly half right.
		//
		// The root credential is the environment variable. Somebody who has it
		// already has the DSN — it is in the same .env file — so there is
		// nothing here they cannot already read. Everyone else gets the fact
		// without the string.
		if s.isRootCredential(r) {
			out["reason"] = s.DegradedReason()
		} else {
			out["reasonWithheld"] = true
		}
		// Degraded is entered once and never left — the only way out is a
		// restart. Saying so here is what stops somebody hunting for a "retry
		// connection" button that deliberately does not exist.
		out["recovery"] = "restart the process after fixing the configuration; degraded is a boot-time state and does not clear itself"
		s.writeJSON(w, http.StatusOK, out)
		return
	}

	// Not degraded at boot does not mean healthy now: the database can go away
	// while the process runs, and that is a different state from both of the
	// others. The overview needs three, not two.
	if err := s.store.Ping(r.Context()); err != nil {
		out["dbReachable"] = false
		if s.isRootCredential(r) {
			out["reason"] = err.Error()
		} else {
			out["reasonWithheld"] = true
		}
		s.writeJSON(w, http.StatusOK, out)
		return
	}
	out["dbReachable"] = true
	s.writeJSON(w, http.StatusOK, out)
}
