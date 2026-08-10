package server

import (
	"context"
	"net/http"
	"time"

	"daycore/internal/ai"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (models)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/models", s.handleAdminModelsGet)
		mux.HandleFunc("POST /api/admin/models/{id}/test", s.handleAdminModelTest)
		mux.HandleFunc("GET /api/admin/oauth", s.handleAdminOAuthGet)
	})
}

// The console's model and OAuth screens.
//
// # Both are READ-ONLY, and that is a decision rather than an omission
//
// The rule this repository applies per field — did the process already build
// something out of it? — puts every field on these two screens on the boot
// side. A model entry became a constructed provider with its base URL and key
// baked in; an OAuth provider became a configured flow with its redirect URI
// registered at the vendor. Neither can change under a running process.
//
// So what is left that a console COULD write?
//
//   - **Which model fills which role** is already editable, through
//     `PUT /api/admin/config` (DEFAULT_CHAT_MODEL and friends). Adding a second
//     door to the same setting would be a second door that can disagree.
//   - **Disabling a model entry** was considered and deliberately not built. It
//     is the one write that can take a deployment down from a web page with no
//     obvious way back: turn off the entry DEFAULT_CHAT_MODEL points at and
//     every AI feature fails, on a screen whose next most likely action is to
//     turn off another one. The recovery that already exists is better — point
//     DEFAULT_CHAT_MODEL somewhere else, which is one runtime setting away and
//     leaves the broken entry visible rather than hidden.
//   - **Disabling an OAuth provider** has the same shape and a weaker case: a
//     provider with an empty client_id is already inert, and the file is the
//     only place its credentials live anyway.
//
// If somebody later wants an enable bit here, the thing to build is the
// consumer first — a Catalog that honours it and refuses to disable the entry a
// default points at. A stored bit nothing reads is this repository's most
// repeated mistake.
//
// # What these screens are FOR
//
// Answering the two questions that setup actually fails on, neither of which is
// answerable from a browser today: **is the key set**, and **which URL do I
// register with the vendor**. Plus one action — a real call to a model, because
// "is this reachable with this key" cannot be answered by looking at
// configuration at all.

var (
	keyAdminModelUnknown = i18n.Reg("admin.models.unknown", i18n.Text{
		"zh-CN": "目录里没有 %s 这个模型",
		"en-US": "There is no model %s in the catalog",
	})
	keyAdminModelTestOK = i18n.Reg("admin.models.test.ok", i18n.Text{
		"zh-CN": "连通，用时 %dms",
		"en-US": "Reachable, %dms",
	})
	keyAdminModelTestFail = i18n.Reg("admin.models.test.fail", i18n.Text{
		"zh-CN": "调不通：%s",
		"en-US": "Call failed: %s",
	})
)

// GET /api/admin/models — the catalog as it is actually loaded.
//
// From the loaded Catalog rather than by re-reading models.yaml. A screen built
// from the file would describe a file that may have changed since boot — that
// is, a process that does not exist. This describes the one that is running.
func (s *Server) handleAdminModelsGet(w http.ResponseWriter, r *http.Request) {
	if s.catalog == nil {
		s.writeJSON(w, http.StatusOK, map[string]any{"models": []any{}})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"models": s.catalog.Details(),
		// The file to edit, since nothing on this screen is writable. Without it
		// the read-only rows are a dead end — the same reason the providers
		// screen reports its config path.
		"configPath": s.cfg.ModelsConfigPath,
	})
}

// modelTestTimeout bounds one connectivity check.
//
// Short on purpose: somebody is watching a spinner. A check that can hang for
// the full AI request timeout is indistinguishable from a crashed console, and
// "it took 30 seconds and then said no" answers the same question as "it took
// 6 seconds and said no".
const modelTestTimeout = 6 * time.Second

// POST /api/admin/models/{id}/test — actually call the model.
//
// # Why this exists when the configuration is right there on the screen
//
// "Is the key set" and "is the key VALID" are different questions, and only the
// second one matters. Nothing about a correct-looking configuration tells you
// that a key was revoked, that a gateway moved, or that the account ran out of
// credit — and today the only way to find out is to use the app and watch a
// feature fail, which reports the failure to a user rather than an operator.
//
// # Boundary: one tiny call, and the reply is thrown away
//
// Max tokens is 1 and the answer is discarded. This is a reachability probe,
// not a playground: a console that can send arbitrary prompts to a model on the
// deployment's key is a way to spend somebody's money from a web page, and it
// is not what the operator came here to do.
func (s *Server) handleAdminModelTest(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	id := r.PathValue("id")
	if s.catalog == nil {
		s.writeErrf(w, locale, http.StatusBadRequest, "unknown_model", keyAdminModelUnknown, id)
		return
	}
	p, ok := s.catalog.Provider(id)
	if !ok {
		s.writeErrf(w, locale, http.StatusBadRequest, "unknown_model", keyAdminModelUnknown, id)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), modelTestTimeout)
	defer cancel()
	started := time.Now()
	_, err := p.Chat(ctx, ai.ChatRequest{
		Messages:  []ai.Message{{Role: ai.RoleUser, Content: "ping"}},
		MaxTokens: 1,
	})
	elapsed := time.Since(started).Milliseconds()

	// A failure is 200 with ok:false, not an HTTP error status.
	//
	// The request to THIS server succeeded — it did what it was asked and found
	// out something. Returning 502 would make the console show a generic "the
	// admin API is broken" banner over a result that is, in fact, the answer.
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "model": id, "elapsedMs": elapsed,
			// The provider's error text, which may quote a base URL. That is the
			// operator's own configuration on an authenticated screen, so it is
			// the one place it belongs — unlike a tool failure, which travels
			// into a model's context and out to a user.
			"message": i18n.Tf(keyAdminModelTestFail, locale, err.Error()),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "model": id, "elapsedMs": elapsed,
		"message": i18n.Tf(keyAdminModelTestOK, locale, elapsed),
	})
}

// GET /api/admin/oauth — which social logins are configured, and the callback
// URL each vendor must have registered.
//
// The redirect URI is the point of this screen. A mismatch there is the most
// common OAuth setup failure, the vendor's error message says nothing useful,
// and the value is derived from PUBLIC_BASE_URL — a config an operator cannot
// see from a browser. It is produced by the same function the login flow uses,
// so it cannot drift into being merely plausible.
func (s *Server) handleAdminOAuthGet(w http.ResponseWriter, r *http.Request) {
	views := []any{}
	if s.oauth != nil {
		for _, v := range s.oauth.Views() {
			views = append(views, v)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"providers":  views,
		"configPath": s.cfg.OAuthConfigPath,
		// Providers with an empty client_id were skipped at load, so this screen
		// cannot show them — saying so beats letting an operator conclude their
		// file was ignored entirely.
		"note": "providers with an empty client_id are skipped at startup and do not appear here",
	})
}
