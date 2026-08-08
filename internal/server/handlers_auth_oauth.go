package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"daycore/internal/auth"
	"daycore/internal/domain"
)

func init() {
	registerRoutes("auth", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/auth/providers", s.handleOAuthProviders)
		mux.HandleFunc("GET /api/auth/oauth/{provider}", s.handleOAuthStart)
		mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.handleOAuthCallback)
	})
}

// GET /api/auth/providers — list enabled OAuth providers.
func (s *Server) handleOAuthProviders(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"providers": s.oauth.Providers()})
}

// GET /api/auth/oauth/{provider} — start the authorization-code flow.
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if !s.oauth.Enabled(provider) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "unknown_provider", "err.oAuthStart.unknown_provider")
		return
	}
	state, err := auth.NewState()
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.oAuthStart.internal")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dc_oauth_state",
		Value:    s.cookies.Sign(provider + "|" + state),
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	url, err := s.oauth.AuthCodeURL(provider, state)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.oAuthStart.internal")
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// GET /api/auth/oauth/{provider}/callback — complete the flow, set the session.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	q := r.URL.Query()
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.oAuthCallback.bad_request")
		return
	}
	c, err := r.Cookie("dc_oauth_state")
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_state", "err.oAuthCallback.bad_state")
		return
	}
	val, ok := s.cookies.Verify(c.Value)
	if !ok || val != provider+"|"+state {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_state", "err.oAuthCallback.bad_state2")
		return
	}
	// consume the state cookie
	http.SetCookie(w, &http.Cookie{Name: "dc_oauth_state", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})

	ctx := r.Context()
	ou, err := s.oauth.Exchange(ctx, provider, code)
	if err != nil {
		s.log.Error("oauth exchange", "provider", provider, "err", err)
		s.writeErrL(w, s.requestLocale(r), http.StatusBadGateway, "oauth_failed", "err.oAuthCallback.oauth_failed")
		return
	}
	user, err := s.upsertOAuthUser(ctx, provider, ou)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.oAuthCallback.internal")
		return
	}
	if _, err := s.issueAndLink(w, r, user); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.oAuthCallback.internal")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound) // back to the frontend
}

func (s *Server) upsertOAuthUser(ctx context.Context, provider string, ou *auth.OAuthUser) (*domain.User, error) {
	if id, err := s.store.Auth().GetOAuthIdentity(ctx, provider, ou.ProviderUserID); err == nil {
		return s.store.Users().GetByID(ctx, id.UserID)
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	var user *domain.User
	email := strings.ToLower(strings.TrimSpace(ou.Email))
	// Only link to an existing account by email when the provider asserts the
	// email is verified — otherwise a provider returning an attacker-chosen
	// unverified address could take over a victim's password account.
	verified := email != "" && ou.EmailVerified
	if verified {
		if u, e := s.store.Users().GetByEmail(ctx, email); e == nil {
			user = u
		}
	}
	if user == nil {
		// For unverified emails, create the account WITHOUT the email so it can
		// never collide with or shadow a real account; identity is keyed only by
		// the provider's subject id below.
		var emailPtr *string
		if verified {
			emailPtr = optStr(email)
		}
		u, err := s.store.Users().Upsert(ctx, &domain.User{
			Email: emailPtr, Name: optStr(ou.Name), AvatarURL: optStr(ou.AvatarURL),
		})
		if err != nil {
			return nil, err
		}
		user = u
	}
	_ = s.store.Auth().CreateOAuthIdentity(ctx, &domain.OAuthIdentity{
		UserID: user.ID, Provider: provider, ProviderUserID: ou.ProviderUserID,
	})
	return user, nil
}

func optStr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
