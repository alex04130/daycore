package server

import (
	"net/http"
	"strings"
	"sync"

	"daycore/internal/i18n"
)

// Degraded boot: the process starts and says what is wrong, instead of dying.
//
// # What it replaces
//
// A store that will not open, or migrations that will not run, used to return an
// error from run() and take the process with it. Under a supervisor that is a
// crash loop: the container restarts every few seconds, the only evidence is a
// line in a log somebody has to know to look for, and there is no way to reach
// the thing that would let you FIX the configuration — because the configuration
// screen is served by the process that keeps dying.
//
// # The rule
//
// Degraded is a boot-time state, entered once and never left. The alternative —
// watching the database and flipping in and out — is a bigger design (what
// happens to a request mid-flip, how many failures count, how a flap is damped)
// and every part of it is a way to be wrong intermittently. A process that
// noticed its database came back would also have to re-run migrations, and doing
// that on a live server is precisely the operation that wants a human. So: it
// starts degraded or it does not, and getting out is a restart.
//
// # What is served
//
// Only what genuinely needs no database, which today is a short list. The value
// is not the list; it is that the process is UP and answering "here is what is
// broken" at an address the operator already knows, and that the console's
// authentication path was built (F4a) to work without a row — so when the
// configuration endpoints land they will work here.
//
// # Health
//
// /api/healthz answers 503. That is deliberate and it is a READINESS answer: a
// load balancer must stop sending user traffic here. Wiring it as a LIVENESS
// probe would make the orchestrator restart the process, which is the crash loop
// this exists to replace — the deployment docs say so next to the endpoint.
type degraded struct {
	mu     sync.RWMutex
	active bool
	// reason is for the operator, through an authenticated path only. A driver
	// error routinely contains the DSN, and a DSN routinely contains a password.
	reason string
}

// EnterDegraded records that storage is unavailable and that this process will
// serve only what needs no database.
func (s *Server) EnterDegraded(reason string) {
	s.degraded.mu.Lock()
	s.degraded.active, s.degraded.reason = true, reason
	s.degraded.mu.Unlock()
}

// Degraded reports whether this process booted without usable storage.
func (s *Server) Degraded() bool {
	if s == nil {
		return false
	}
	s.degraded.mu.RLock()
	defer s.degraded.mu.RUnlock()
	return s.degraded.active
}

// DegradedReason is the detail, for authenticated callers only.
func (s *Server) DegradedReason() string {
	s.degraded.mu.RLock()
	defer s.degraded.mu.RUnlock()
	return s.degraded.reason
}

// degradedRoutes are the paths that still answer when storage is gone.
//
// A prefix list rather than exact patterns, because the point is "these
// subtrees", and because a new admin endpoint should inherit the behaviour
// rather than have to remember to ask for it.
//
// ⚠️ Everything under /api/admin is listed, and most of it will still fail —
// the prompt overrides, the stats and the DB browser all read rows. They fail
// with their own honest error rather than a blanket 503, which is the more
// useful answer: "this particular screen needs the database" beats "the API is
// down" when the operator is standing in front of a console that loaded.
var degradedRoutes = []string{
	"/api/healthz", // must answer, and answers 503
	"/api/version", // contract negotiation; reads no rows
	"/api/admin/",  // the console, including its login (F4a built it DB-free)
}

var keyDegraded = i18n.Reg("server.degraded", i18n.Text{
	"zh-CN": "存储不可用，这个进程只能提供管理面。看启动日志或 /api/healthz。",
	"en-US": "Storage is unavailable; this process is serving the admin console only. See the startup log or /api/healthz.",
})

// degradedMW short-circuits everything that would touch the database.
//
// It runs before the session middleware rather than inside the handlers: with no
// store, requestLocale and the session lookup would dereference nil, and a
// panic caught by recoverMW is a 500 that says nothing. Refusing early gives one
// answer, with a reason, at every affected path.
func (s *Server) degradedMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Degraded() || degradedAllowed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// The deployment default locale, not the session's — there is no session
		// to read, which is the whole problem.
		locale := s.defaultLocales.Resolve("", r.Header.Get("Accept-Language"))
		w.Header().Set("Retry-After", "0") // a restart, not a wait
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyDegraded, locale))
	})
}

func degradedAllowed(path string) bool {
	for _, p := range degradedRoutes {
		if path == strings.TrimSuffix(p, "/") || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
