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

	"daycore/internal/ai"
	"daycore/internal/auth"
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
	Logger   *slog.Logger
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
	search      *search.Client
	searcher    domain.Searcher
	decisions   *decisionRegistry
	worker      *Worker // set by main.go after construction; nil when channels are off

	// asyncWG tracks detached background goroutines (async companion turns,
	// inbound channel handling) so graceful shutdown can wait for them.
	asyncWG sync.WaitGroup
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
		weather:     d.Weather, search: search.New(), searcher: d.Searcher,
		decisions: newDecisionRegistry(),
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

	// health & diagnostics
	mux.HandleFunc("GET /api/healthz", s.handleHealth)
	mux.HandleFunc("GET /api/version", s.handleAPIVersion)
	mux.HandleFunc("GET /api/models", s.handleModels)

	// session
	mux.HandleFunc("POST /api/session/init", s.handleSessionInit)
	mux.HandleFunc("POST /api/session/theme", s.handleSessionTheme)
	mux.HandleFunc("PATCH /api/session/settings", s.handleSessionSettings)
	mux.HandleFunc("GET /api/session/preferences", s.handleSessionGetPreferences)
	mux.HandleFunc("PATCH /api/session/preferences", s.handleSessionPreferences)

	// plans
	mux.HandleFunc("GET /api/plan", s.handlePlanGet)
	mux.HandleFunc("POST /api/plan", s.handlePlanUpsert)
	mux.HandleFunc("PATCH /api/plan", s.handlePlanPatch)
	mux.HandleFunc("GET /api/plan/range", s.handlePlanRange)

	// schedule rules
	mux.HandleFunc("GET /api/rules", s.handleRuleList)
	mux.HandleFunc("POST /api/rules", s.handleRuleCreate)
	mux.HandleFunc("POST /api/rules/batch", s.handleRuleBatchCreate)
	mux.HandleFunc("PATCH /api/rules/{id}", s.handleRulePatch)
	mux.HandleFunc("DELETE /api/rules/{id}", s.handleRuleDelete)

	// moods
	mux.HandleFunc("GET /api/mood", s.handleMoodList)
	mux.HandleFunc("POST /api/mood", s.handleMoodCreate)
	mux.HandleFunc("PATCH /api/mood", s.handleMoodPatch)

	// companion history
	mux.HandleFunc("GET /api/companion-history", s.handleCompanionHistoryGet)
	mux.HandleFunc("POST /api/companion-history", s.handleCompanionHistorySet)

	// feedback
	mux.HandleFunc("POST /api/feedback", s.handleFeedbackAdd)

	// imports
	mux.HandleFunc("GET /api/import/token", s.handleImportTokenGet)
	mux.HandleFunc("POST /api/import/token", s.handleImportTokenRotate)
	mux.HandleFunc("POST /api/import/canvas", s.handleImportCanvas)
	mux.HandleFunc("POST /api/import/ics", s.handleImportICS)

	// inbox
	mux.HandleFunc("POST /api/inbox/process", s.handleInboxProcess)
	mux.HandleFunc("POST /api/inbox/upload", s.handleInboxUpload)
	mux.HandleFunc("POST /api/inbox/commit", s.handleInboxCommit)

	// temp-context (session-scoped key-value store)
	mux.HandleFunc("GET /api/temp-context", s.handleTempContextGet)
	mux.HandleFunc("PUT /api/temp-context", s.handleTempContextPut)

	// long-term memory
	mux.HandleFunc("GET /api/memory", s.handleMemoryList)
	mux.HandleFunc("POST /api/memory", s.handleMemoryAdd)
	mux.HandleFunc("DELETE /api/memory", s.handleMemoryClear)
	mux.HandleFunc("DELETE /api/memory/{id}", s.handleMemoryDelete)
	mux.HandleFunc("GET /api/import/history", s.handleImportHistory)

	// custom themes
	mux.HandleFunc("GET /api/themes", s.handleThemeList)
	mux.HandleFunc("POST /api/themes", s.handleThemeCreate)
	mux.HandleFunc("PATCH /api/themes/{id}", s.handleThemePatch)
	mux.HandleFunc("DELETE /api/themes/{id}", s.handleThemeDelete)

	// canvas materials
	mux.HandleFunc("GET /api/courses", s.handleCourseList)
	mux.HandleFunc("GET /api/assignments", s.handleAssignmentList)
	mux.HandleFunc("POST /api/assignments", s.handleAssignmentCreate)
	mux.HandleFunc("PATCH /api/assignments/{id}", s.handleAssignmentPatch)

	// materials
	mux.HandleFunc("GET /api/materials", s.handleMaterialList)
	mux.HandleFunc("POST /api/materials", s.handleMaterialCreate)
	mux.HandleFunc("GET /api/materials/search", s.handleMaterialSearch)
	mux.HandleFunc("GET /api/materials/categories", s.handleMaterialCategories)
	mux.HandleFunc("GET /api/materials/{id}", s.handleMaterialGet)
	mux.HandleFunc("PATCH /api/materials/{id}", s.handleMaterialUpdate)
	mux.HandleFunc("DELETE /api/materials/{id}", s.handleMaterialDelete)

	// wishes
	mux.HandleFunc("GET /api/wishes", s.handleWishList)
	mux.HandleFunc("POST /api/wishes", s.handleWishCreate)
	mux.HandleFunc("GET /api/wishes/{id}", s.handleWishGet)
	mux.HandleFunc("PATCH /api/wishes/{id}", s.handleWishUpdate)
	mux.HandleFunc("DELETE /api/wishes/{id}", s.handleWishDelete)

	// ai
	mux.HandleFunc("POST /api/ai/plan-text", s.handleAIPlanText)
	mux.HandleFunc("POST /api/ai/plan-image", s.handleAIPlanImage)
	mux.HandleFunc("POST /api/ai/extract-schedule-image", s.handleAIExtractScheduleImage)
	mux.HandleFunc("POST /api/ai/auto-plan", s.handleAIAutoPlan)
	mux.HandleFunc("POST /api/ai/theme", s.handleAITheme)
	mux.HandleFunc("POST /api/ai/mood", s.handleAIMood)
	mux.HandleFunc("POST /api/ai/travel", s.handleAITravel)
	mux.HandleFunc("POST /api/ai/companion", s.handleAICompanion)
	mux.HandleFunc("POST /api/ai/companion/async", s.handleAICompanionAsync)
	mux.HandleFunc("POST /api/decisions/{id}/respond", s.handleDecisionRespond)

	// chat threads
	mux.HandleFunc("GET /api/chat/threads", s.handleChatListThreads)
	mux.HandleFunc("POST /api/chat/threads", s.handleChatCreateThread)
	mux.HandleFunc("PATCH /api/chat/threads/{id}", s.handleChatUpdateThread)
	mux.HandleFunc("DELETE /api/chat/threads/{id}", s.handleChatDeleteThread)
	mux.HandleFunc("GET /api/chat/threads/{id}/messages", s.handleChatListMessages)
	mux.HandleFunc("DELETE /api/chat/threads/{id}/messages", s.handleChatClearMessages)
	mux.HandleFunc("GET /api/chat/messages/{id}", s.handleChatGetMessage)

	// operation logs & undo
	mux.HandleFunc("GET /api/ops", s.handleOpList)
	mux.HandleFunc("POST /api/ops/{id}/revert", s.handleOpRevert)

	// auth
	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/providers", s.handleOAuthProviders)
	mux.HandleFunc("GET /api/auth/oauth/{provider}", s.handleOAuthStart)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.handleOAuthCallback)
	mux.HandleFunc("GET /api/me", s.handleMe)

	// admin (prompts)
	mux.HandleFunc("GET /api/channels", s.handleChannelList)
	mux.HandleFunc("POST /api/channels/{channel}/bind", s.handleChannelBind)
	mux.HandleFunc("POST /api/channels/{channel}/verify", s.handleChannelVerify)
	mux.HandleFunc("DELETE /api/channels/{channel}/unbind", s.handleChannelUnbind)

	// admin (prompts)
	mux.HandleFunc("GET /api/admin/prompts", s.handleAdminPromptList)
	mux.HandleFunc("GET /api/admin/prompts/{key}", s.handleAdminPromptGet)
	mux.HandleFunc("PUT /api/admin/prompts/{key}", s.handleAdminPromptSet)

	// admin (stats, users, DB)
	mux.HandleFunc("GET /api/admin/stats", s.handleAdminStats)
	mux.HandleFunc("GET /api/admin/ailogs", s.handleAdminAILogs)
	mux.HandleFunc("GET /api/admin/users", s.handleAdminUsers)
	mux.HandleFunc("DELETE /api/admin/users/{id}", s.handleAdminDeleteUser)
	mux.HandleFunc("GET /api/admin/db/tables", s.handleAdminDBTables)
	mux.HandleFunc("GET /api/admin/db/table/{name}", s.handleAdminDBTableBrowse)
	mux.HandleFunc("DELETE /api/admin/db/table/{name}/{id}", s.handleAdminDBTableDelete)
	mux.HandleFunc("GET /api/admin/db/export", s.handleAdminDBExport)
	mux.HandleFunc("POST /api/admin/db/import", s.handleAdminDBImport)
	mux.HandleFunc("GET /api/admin/db/backup", s.handleAdminDBBackup)

	// static frontend (SPA)
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

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (string, bool) {
	sid := sessionIDFrom(r.Context())
	if sid == "" {
		s.writeErr(w, http.StatusUnauthorized, "no_session", "缺少会话，请先初始化会话")
		return "", false
	}
	return sid, true
}

func (s *Server) requestLocale(r *http.Request) string {
	sessionLang := ""
	if sid := sessionIDFrom(r.Context()); sid != "" {
		if sess, err := s.store.Sessions().Get(r.Context(), sid); err == nil {
			sessionLang = sess.Language
		}
	}
	return i18n.Resolve(sessionLang, r.Header.Get("Accept-Language"))
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
