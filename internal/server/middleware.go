package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "err", rec, "path", r.URL.Path, "rid", requestIDFrom(r.Context()))
				s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.recoverMW.internal")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestIDMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loggingMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		s.log.Info("http",
			"method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds(), "ip", s.clientIP(r), "rid", requestIDFrom(r.Context()))
	})
}

func (s *Server) corsMW(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	wildcard := false
	for _, o := range s.cfg.AllowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/import/"):
			// The browser extension pushes cross-origin with X-Import-Token
			// (token auth, no cookies), so any origin is safe to allow here.
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Import-Token, X-Request-ID")
			}
		case origin != "" && wildcard:
			// Wildcard allowlist: reflect the origin but NEVER combine it with
			// credentials — that would let any site make cookie-bearing calls and
			// read the response on a logged-in victim's behalf.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token, X-Request-ID, Authorization, X-Session-Token")
		case origin != "" && allowed[origin]:
			// Explicitly allowlisted origin: safe to allow credentials.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token, X-Request-ID, Authorization, X-Session-Token")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sessionMW resolves the anonymous session into ctxSessionID (if valid). Two
// carriers, same signed value and same verification: the X-Session-Token header
// (native apps / separated frontends, checked first) and the dc_sid cookie.
func (s *Server) sessionMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(sessionTokenHeader)
		if token == "" {
			if c, err := r.Cookie(sessionCookie); err == nil {
				token = c.Value
			}
		}
		if token != "" {
			if sid, ok := s.cookies.Verify(token); ok {
				r = r.WithContext(context.WithValue(r.Context(), ctxSessionID, sid))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the JWT from an "Authorization: Bearer <jwt>" header, or
// returns "" when absent/malformed.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// userMW resolves the auth JWT into ctxUserID + ctxUser (if valid). Two
// carriers, same token and same checks: the Authorization Bearer header
// (native apps / separated frontends, checked first) and the dc_auth cookie.
// The token's version claim must match the user's current token_version, so a
// logout (which bumps the version) invalidates every previously issued JWT. The
// resolved user is stashed for dataSessionMW to reuse (no second DB lookup).
func (s *Server) userMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			if c, err := r.Cookie(authCookie); err == nil {
				token = c.Value
			}
		}
		if token != "" {
			if uid, ver, err := s.tokens.Parse(token); err == nil {
				if user, err := s.store.Users().GetByID(r.Context(), uid); err == nil && user != nil && user.TokenVersion == ver {
					ctx := context.WithValue(r.Context(), ctxUserID, uid)
					ctx = context.WithValue(ctx, ctxUser, user)
					r = r.WithContext(ctx)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// dataSessionMW resolves the authenticated user's canonical data session from
// the user userMW already fetched.
func (s *Server) dataSessionMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user := userFrom(r.Context()); user != nil && user.DataSessionID != "" {
			r = r.WithContext(context.WithValue(r.Context(), ctxDataSessionID, user.DataSessionID))
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit enforces the per-IP AI limit; writes 429 and returns false if exceeded.
func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request) bool {
	if s.limiter.Allow(s.clientIP(r)) {
		return true
	}
	s.writeErrL(w, s.requestLocale(r), http.StatusTooManyRequests, "rate_limited", "err.rateLimit.rate_limited")
	return false
}

// authRateLimit enforces the per-IP auth-endpoint limit (login/register), which
// is stricter than the AI limit and guards against credential stuffing and
// argon2 memory-amplification DoS. Writes 429 and returns false if exceeded.
func (s *Server) authRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if s.authLimiter.Allow(s.clientIP(r)) {
		return true
	}
	s.writeErrL(w, s.requestLocale(r), http.StatusTooManyRequests, "rate_limited", "err.authRateLimit.rate_limited")
	return false
}

// ─── status-capturing ResponseWriter (Unwrap lets ResponseController flush SSE) ──

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// ─── rate limiter (fixed window per minute) ──────────────────────────────────

type rateLimiter struct {
	mu     sync.Mutex
	perMin int
	hits   map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newRateLimiter(perMin int) *rateLimiter {
	return &rateLimiter{perMin: perMin, hits: map[string]*window{}}
}

func (l *rateLimiter) Allow(ip string) bool {
	if l.perMin <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w := l.hits[ip]
	if w == nil || now.Sub(w.start) > time.Minute {
		l.hits[ip] = &window{start: now, count: 1}
		return true
	}
	if w.count >= l.perMin {
		return false
	}
	w.count++
	return true
}
