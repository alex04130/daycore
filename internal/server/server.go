// Package server wires the HTTP API: routing, middleware, and handlers.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/blob"
	"daycore/internal/config"
	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/search"
)

// Deps are the constructed dependencies the server needs.
type Deps struct {
	Config   *config.Config
	Store    domain.Store
	Catalog  *ai.Catalog
	Vision   *ai.Orchestrator
	Prompts  *ai.PromptService
	Hasher   *auth.Hasher
	Tokens   *auth.TokenIssuer
	Cookies  *auth.CookieSigner
	OAuth    *auth.OAuthManager
	Searcher domain.Searcher
	Weather  domain.WeatherProvider
	// Blobs is the file bus. nil is a supported configuration — every feature
	// that needs bytes checks and says so.
	Blobs  blob.Store
	Logger *slog.Logger
}

// Server holds the dependencies and exposes an http.Handler.
type Server struct {
	cfg         *config.Config
	store       domain.Store
	catalog     *ai.Catalog
	vision      *ai.Orchestrator
	prompts     *ai.PromptService
	hasher      *auth.Hasher
	tokens      *auth.TokenIssuer
	cookies     *auth.CookieSigner
	oauth       *auth.OAuthManager
	log         *slog.Logger
	limiter     *rateLimiter
	authLimiter *rateLimiter
	weather     domain.WeatherProvider
	blobs       blob.Store
	search      *search.Client
	searcher    domain.Searcher
	decisions   *decisionRegistry
	worker      *Worker // set by main.go after construction
	// awake throttles rhythm signal writes. See awake.go — it is why
	// requireSession, and not a list of paths, decides what counts as awake.
	awake *awakeTracker
	// scheduleOnUse gives a session its proactive cron entries the first time it
	// is seen, replacing the boot-time enumeration of channel bindings.
	scheduleOnUse atomic.Pointer[func(sid string)]

	// defaultLocales is the language pair a user starts with before choosing
	// their own. It is a default, not a restriction — see localePair.
	defaultLocales i18n.Pair

	// asyncWG tracks detached background goroutines (async companion turns,
	// inbound channel handling) so graceful shutdown can wait for them.
	asyncWG sync.WaitGroup

	// Background tick loops (see ticker.go). tickOnce lazily creates ticksDone so
	// the zero value of Server stays usable — RouteTable(&Server{}) builds one.
	tickOnce  sync.Once
	tickStop  sync.Once
	ticksDone chan struct{}
	tickWG    sync.WaitGroup
}

// GoTracked runs fn on a goroutine tracked by the background WaitGroup so
// graceful shutdown (WaitBackground) can wait for in-flight agent work.
func (s *Server) GoTracked(fn func()) {
	s.asyncWG.Add(1)
	go func() {
		defer s.asyncWG.Done()
		fn()
	}()
}

// WaitBackground blocks until every tracked background goroutine finishes or
// ctx expires (main.go calls it after the HTTP server drains).
func (s *Server) WaitBackground(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.asyncWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetWorker attaches the background worker so handlers (e.g. channel verify) can
// schedule proactive jobs for a session on demand.
func (s *Server) SetWorker(w *Worker) { s.worker = w }

// New builds a Server.
func New(d Deps) *Server {
	return &Server{
		cfg: d.Config, store: d.Store, catalog: d.Catalog, vision: d.Vision,
		prompts: d.Prompts, hasher: d.Hasher, tokens: d.Tokens, cookies: d.Cookies,
		oauth: d.OAuth, log: d.Logger, limiter: newRateLimiter(d.Config.RateLimitPerMin),
		authLimiter: newRateLimiter(d.Config.AuthRateLimitPerMin),
		weather:     d.Weather, blobs: d.Blobs, search: search.New(), searcher: d.Searcher,
		awake:          newAwakeTracker(),
		decisions:      newDecisionRegistry(),
		defaultLocales: d.Config.DefaultLocales,
	}
}

const (
	sessionCookie = "dc_sid"
	authCookie    = "dc_auth"

	// sessionTokenHeader carries the same signed value as the dc_sid cookie for
	// clients without a cookie jar (native apps, separated frontends). A custom
	// header always triggers a CORS preflight, so this path is CSRF-immune.
	sessionTokenHeader = "X-Session-Token"
)

// Handler builds the routed, middleware-wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Routes are registered by the files that own their handlers (see routes.go).
	// This used to be one hundred mux.HandleFunc calls in this function, which
	// made it the single most contended file in the repo — twelve parallel work
	// items all had to edit the same list.
	for _, g := range routeGroups {
		g.register(s, mux)
	}

	// static frontend (SPA) — stays here because it is conditional on
	// STATIC_DIR existing, and because "/" must be registered by exactly one
	// thing. Go 1.22's ServeMux resolves by specificity rather than by
	// registration order, so "/" loses to every real route regardless of when it
	// goes in.
	if h := s.staticHandler(); h != nil {
		mux.Handle("/", h)
	}

	// middleware chain (outermost first)
	return s.recoverMW(s.requestIDMW(s.loggingMW(s.corsMW(s.sessionMW(s.userMW(s.dataSessionMW(mux)))))))
}

// ─── context plumbing ────────────────────────────────────────────────────────

type ctxKey int

const (
	ctxSessionID ctxKey = iota
	ctxUserID
	ctxRequestID
	ctxDataSessionID
	ctxUser
)

func sessionIDFrom(ctx context.Context) string {
	if v, _ := ctx.Value(ctxDataSessionID).(string); v != "" {
		return v
	}
	v, _ := ctx.Value(ctxSessionID).(string)
	return v
}

func userIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxUserID).(string)
	return v
}

// userFrom returns the authenticated user resolved by userMW (nil if none).
func userFrom(ctx context.Context) *domain.User {
	v, _ := ctx.Value(ctxUser).(*domain.User)
	return v
}

func requestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestID).(string)
	return v
}

// requireSession is the gate on every user-facing data endpoint — and therefore
// also the definition of "the user was awake": machine pushes authenticate
// through importSession instead, and inbound channel messages never reach an
// HTTP handler. See awake.go.
func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	sid := sessionIDFrom(r.Context())
	if sid == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "no_session", "err.requireSession.no_session")
		return "", false
	}
	s.markAwake(sid)
	return sid, true
}

func (s *Server) requestLocale(r *http.Request) string {
	sid := sessionIDFrom(r.Context())
	if sid == "" {
		return s.defaultLocales.Resolve("", r.Header.Get("Accept-Language"))
	}
	sess, err := s.store.Sessions().Get(r.Context(), sid)
	if err != nil {
		return s.defaultLocales.Resolve("", r.Header.Get("Accept-Language"))
	}
	return s.localePair(r.Context(), sid).Resolve(sess.Language, r.Header.Get("Accept-Language"))
}

// localePair is the two languages this user's switch toggles between: their own
// choice where they made one, the deployment default for the rest.
func (s *Server) localePair(ctx context.Context, sid string) i18n.Pair {
	prefs := s.sessionPrefs(ctx, sid)
	return i18n.PairOr(prefs.PrimaryLocale, prefs.SecondaryLocale, s.defaultLocales)
}

// ─── response/request helpers ────────────────────────────────────────────────

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeErr(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]string{"error": code, "message": message})
}

// writeErrL is writeErr with a translatable message.
//
// Every user-visible error in this package goes through it. The plain writeErr
// above stays for the two cases where the text is genuinely not a message to a
// person — and messages_test.go fails the build if a new caller passes a
// literal containing Chinese, because "remember to use the catalog" is not a
// mechanism and this repo has been bitten by exactly that kind of rule before.
//
// The locale is passed rather than derived from a request: the revert handlers
// have no *http.Request, and threading one into them purely to look up a
// language would put HTTP in a layer that had managed to stay out of it.
func (s *Server) writeErrL(w http.ResponseWriter, locale string, status int, code, key string) {
	s.writeJSON(w, status, map[string]string{"error": code, "message": i18n.T(key, locale)})
}

func (s *Server) readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 12<<20))
	return dec.Decode(dst)
}

// clientIP resolves the client IP used for rate limiting and logs.
// X-Forwarded-For is only honored when TrustProxyHeaders is set (the app sits
// behind a trusted reverse proxy); otherwise a client could spoof the header to
// land in a fresh per-IP bucket and defeat the AI limiter. When trusted, the
// last hop is used — the address the trusted proxy actually observed, which the
// client cannot forge by prepending its own XFF value.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// sameSiteMode maps cfg.CookieSameSite to the http constant. "none" (cross-site
// cookie deployments) is validated at config load to require Secure.
func (s *Server) sameSiteMode() http.SameSite {
	switch s.cfg.CookieSameSite {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.cookies.Sign(sessionID),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: s.sameSiteMode(),
		MaxAge:   400 * 24 * 3600,
	})
}

func (s *Server) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: s.sameSiteMode(),
		MaxAge:   int(s.cfg.JWTTTL.Seconds()),
	})
}

func (s *Server) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: authCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: s.cfg.SecureCookies, SameSite: s.sameSiteMode(), MaxAge: -1,
	})
}

// writeErrf is writeErrL for a message that carries a value.
//
// A format string rather than concatenation at the call site: a translator has
// to be able to move the placeholder, and "prefix" + value cannot express a
// language that puts the noun first.
func (s *Server) writeErrf(w http.ResponseWriter, locale string, status int, code, key string, args ...any) {
	s.writeJSON(w, status, map[string]string{"error": code, "message": i18n.Tf(key, locale, args...)})
}
