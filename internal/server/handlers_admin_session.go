package server

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"daycore/internal/auth"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (session)", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/admin/session", s.handleAdminLogin)
		mux.HandleFunc("DELETE /api/admin/session", s.handleAdminLogout)
	})
}

// Two ways in, for two different callers.
//
//	X-Admin-Token   machines: curl, CI, a deploy script. Unchanged, and it was
//	                never the problem — the token is in a header, not a URL, and
//	                the comparison is already constant-time.
//	dc_admin cookie humans: the console. httpOnly, so an XSS on the console page
//	                cannot read it; Secure and SameSite=Strict; short-lived.
//
// # What this fixes
//
// The console used to keep the raw ADMIN_TOKEN in sessionStorage and send it as
// a header on every request. Three problems, and only the first is obvious:
//
//  1. Any XSS on the console reads a credential that NEVER EXPIRES.
//  2. It is an environment variable, so rotating it means a redeploy — no TTL,
//     no revocation, no way to say "that session, not all of them".
//  3. Unset meant the admin API was open on any non-production deployment.
//
// (3) is closed in config.Load, which now invents one rather than leaving the
// door open. (1) and (2) are closed here: the raw token is exchanged ONCE for a
// short-lived cookie, and the raw token never enters JavaScript-readable storage
// again.
//
// # CSRF
//
// A cookie is sent by the browser on cross-site requests too, so httpOnly buys
// confidentiality and not authority. Two guards, and neither alone is enough:
//
//	SameSite=Strict  the browser will not attach it to a cross-site request
//	Origin check     for browsers or flows where SameSite is not honoured
//
// The header path needs neither: a custom request header forces a preflight,
// which a cross-origin attacker cannot satisfy.
//
// # Degraded boot
//
// None of this touches the database. That is deliberate and is the constraint
// F4a exists to satisfy: the planned degraded boot serves the console while
// storage is unavailable, so an admin login that needed a row would be an admin
// login that stops working exactly when it is needed. The cost is that there is
// no revocation list — which is why the TTL is short and why raising it is a
// security change (see auth.AdminTokenTTL).
const adminCookie = "dc_admin"

var (
	keyAdminBadToken = i18n.Reg("admin.session.bad_token", i18n.Text{
		"zh-CN": "管理令牌不对",
		"en-US": "That admin token is not right",
	})
	keyAdminNoOrigin = i18n.Reg("admin.session.bad_origin", i18n.Text{
		"zh-CN": "请求来源不被允许",
		"en-US": "That request origin is not allowed",
	})
)

// POST /api/admin/session — exchange the raw admin token for a session cookie.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.sameOriginRequest(r) {
		s.writeErr(w, http.StatusForbidden, "bad_origin", i18n.T(keyAdminNoOrigin, locale))
		return
	}
	if !s.authRateLimit(w, r) {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = s.readJSON(r, &body)

	// Constant-time, and it also has to refuse an empty configured token rather
	// than matching an empty submission — config.Load makes that unreachable,
	// but a guard that depends on another file staying correct is not a guard.
	if s.cfg.AdminToken == "" || subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.cfg.AdminToken)) != 1 {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", i18n.T(keyAdminBadToken, locale))
		return
	}

	tok, err := s.tokens.IssueAdmin("console")
	if err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminLogin.internal")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    tok,
		Path:     "/api/admin",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		// Strict, not Lax, and not the deployment's COOKIE_SAMESITE. The session
		// cookie is Lax so a link into the app still works; nobody should arrive
		// at the admin console by following a link from somewhere else, and the
		// whole value of Strict here is that they cannot.
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(auth.AdminTokenTTL.Seconds()),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"expiresIn": int(auth.AdminTokenTTL.Seconds()),
	})
}

// DELETE /api/admin/session — log out of the console.
//
// Clearing the cookie is all there is, because there is no revocation list. A
// copy of the token taken before logout stays valid until it expires, which is
// the honest cost of an auth path that must work with no database.
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    "",
		Path:     "/api/admin",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sameOriginRequest checks the Origin header against this deployment.
//
// # Behind a reverse proxy, which is the normal deployment
//
// Comparing Origin against r.Host alone is how this check kills a proxied
// install. The browser sends `https://daycore.example.com`; what the process
// sees in r.Host depends entirely on the proxy's configuration, and NGINX'S
// DEFAULT IS WRONG FOR US — `proxy_pass` sets Host to `$proxy_host`, the
// upstream address, so r.Host is `daycore:8080` and every admin request is a
// 403. deploy/nginx.conf sets `Host $host`, but the operator writing their own
// config is the normal case and a security check that depends on somebody
// remembering one line is not a security check.
//
// So the accepted set is built from everything that legitimately identifies this
// deployment:
//
//	PUBLIC_BASE_URL      authoritative. The operator already had to state the
//	                     external URL here for OAuth redirects to work, so it is
//	                     both correct and already verified by another feature.
//	X-Forwarded-Host     the other common proxy convention, and ONLY when
//	                     TRUST_PROXY_HEADERS says the proxy is trusted — an
//	                     untrusted client can set it to anything.
//	r.Host               the direct case, and the proxied case when Host is
//	                     forwarded properly.
//	ALLOWED_ORIGINS      an explicit operator decision.
//
// # Scheme is deliberately ignored
//
// TLS terminates at the proxy, so a request arriving over https reaches this
// process as plain http. Comparing schemes would reject exactly the deployments
// that did TLS correctly. Host and port are what identify the site; the scheme
// downgrade an attacker would need is a network position from which CSRF is the
// least of the problems.
//
// # Absent Origin is allowed
//
// Non-browser clients (curl, CI) do not send one, and they are exactly who the
// header path serves. A browser always sends it on a cross-origin request, which
// is the case being screened.
//
// Deployment note: in a real proxied install the upstream port is not exposed
// anyway (compose does not publish it, bare metal binds loopback), so this needs
// no special care. The thing to avoid is running a proxy AND publishing the app
// port on the same host — then the proxy is optional from the attacker's side,
// and with it go the rate limits and the TLS.
func (s *Server) sameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	for _, allowed := range s.cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimSpace(allowed), origin) {
			return true
		}
	}
	for _, host := range s.deploymentHosts(r) {
		if host != "" && strings.EqualFold(host, u.Host) {
			return true
		}
	}
	return false
}

// deploymentHosts lists the host:port values that legitimately mean "this
// deployment", most authoritative first.
func (s *Server) deploymentHosts(r *http.Request) []string {
	hosts := make([]string, 0, 3)
	if s.cfg != nil && s.cfg.PublicBaseURL != "" {
		if u, err := url.Parse(s.cfg.PublicBaseURL); err == nil {
			hosts = append(hosts, u.Host)
		}
	}
	// Only from a proxy we were told to trust. TRUST_PROXY_HEADERS already gates
	// X-Forwarded-For for the same reason: a header a client can set is a header
	// a client can lie with, and here the lie would be "I am same-origin".
	if s.cfg != nil && s.cfg.TrustProxyHeaders {
		if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); fwd != "" {
			// A proxy chain appends, so the first entry is the original client's.
			if i := strings.IndexByte(fwd, ','); i >= 0 {
				fwd = strings.TrimSpace(fwd[:i])
			}
			hosts = append(hosts, fwd)
		}
	}
	return append(hosts, r.Host)
}
