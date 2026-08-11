package server

import (
	"errors"
	"net/http"
	"strings"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/theme"
)

func init() {
	registerRoutes("admin (theme kinds)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/theme-kinds", s.handleAdminKindList)
		mux.HandleFunc("PUT /api/admin/theme-kinds/{name}", s.handleAdminKindPut)
		mux.HandleFunc("DELETE /api/admin/theme-kinds/{name}", s.handleAdminKindDelete)
	})
}

// The third tier's screen: a validation rule somebody proposed, and a person
// deciding.
//
// # ⚠️ What approving does NOT do
//
// It does not make a value safe. The character floor runs before any kind is
// consulted and again on every leaf of a combinator, so an approved wide-open
// pattern is still a wide-open pattern INSIDE a safe alphabet — there is a test
// that installs a `.*` kind and shows seven injection strings still refused.
//
// What it does is agree to a MEANING. A pattern is a promise about what a token
// may hold, stored themes are validated against it, and a promise nobody read
// is not a promise. That is the whole content of this gate, and saying so
// plainly is the difference between an operator who reads the regex and one who
// clicks because the screen looked scary and they wanted it to go away.
//
// # ⚠️ Approval is bound to the TEXT
//
// Re-proposing a different pattern revokes it automatically (see
// recordProposedKinds). Otherwise a frontend gets `^[0-9]+$` approved and ships
// `.*` the following week with the flag still set.
var (
	keyKindNotFound = i18n.Reg("admin.kinds.not_found", i18n.Text{
		"zh-CN": "没有这个 kind",
		"en-US": "No such kind",
	})
	keyKindBadName = i18n.Reg("admin.kinds.bad_name", i18n.Text{
		"zh-CN": "kind 的名字只能是小写字母开头的字母、数字和连字符，而且不能是组合子或内置原语",
		"en-US": "A kind name must be lowercase letters, digits and hyphens starting with a letter, and may not be a combinator or a built-in primitive",
	})
	keyKindBadPattern = i18n.Reg("admin.kinds.bad_pattern", i18n.Text{
		"zh-CN": "这个正则编不过或者太长：",
		"en-US": "This pattern will not compile, or is too long: ",
	})
	keyKindDegraded = i18n.Reg("admin.kinds.degraded", i18n.Text{
		"zh-CN": "数据库不可用，kind 读不到也改不了。内置的六种和 THEME_KINDS_DIR 仍然在生效。",
		"en-US": "The database is unavailable, so kinds cannot be read or changed. The built-in six and THEME_KINDS_DIR are still in force.",
	})
	keyKindInternal = i18n.Reg("admin.kinds.internal", i18n.Text{
		"zh-CN": "kind 读写失败",
		"en-US": "Could not read or write the kind",
	})
)

// GET /api/admin/theme-kinds — every stored kind, plus what is actually in
// force and what each row would shadow.
func (s *Server) handleAdminKindList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyKindDegraded, s.requestLocale(r)))
		return
	}
	rows, err := s.store.ThemeKinds().List(r.Context())
	if err != nil {
		s.log.Error("theme kind list", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyKindInternal, s.requestLocale(r)))
		return
	}
	out := make([]kindView, 0, len(rows))
	pending := 0
	for _, k := range rows {
		if !k.Approved {
			pending++
		}
		out = append(out, kindView{
			ThemeKind: k,
			// ⚠️ "In force" is NOT the same as "approved". A row can be approved
			// and still not be validating anything, because its pattern stopped
			// compiling or grew past the bound and the loader skipped it. That
			// state — approved with nothing enforcing it — is the one an
			// operator most needs to see, and the one a single boolean hides.
			InForce: k.Approved && s.themeKinds.OriginOf(k.Name) == theme.OriginDB,
			Shadows: s.themeKinds.BaseOrigin(k.Name),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"kinds":   out,
		"pending": pending,
		// The ceiling on unauthenticated proposals, reported rather than
		// hardcoded in the console — same rule as every other number that screen
		// states about this deployment.
		"maxPending": domain.MaxProposedKinds,
		"maxPattern": theme.MaxStoredPattern,
		// What is running right now, whatever its layer. The screen shows the
		// stored rows; this is what the deployment actually validates against,
		// and the two are not the same list.
		"inForce": s.themeKindViews(),
	})
}

type kindInput struct {
	Pattern     *string `json:"pattern"`
	Description *string `json:"description"`
	Approved    *bool   `json:"approved"`
}

// PUT /api/admin/theme-kinds/{name} — approve, revoke, or write one outright.
//
// ⚠️ Three-state fields, like the family screen and for the same reason: "leave
// the pattern alone" and "set the pattern to empty" are different requests, and
// an interface that could only express the second would wipe a pattern every
// time somebody approved something.
func (s *Server) handleAdminKindPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyKindDegraded, locale))
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if !proposedKindNameOK(name) {
		// ⚠️ The same rule the handshake applies, and deliberately so: an
		// operator MAY redefine a primitive, but only through THEME_KINDS_DIR —
		// a file on the machine they already have. Doing it from a web form
		// would mean an XSS or a stolen console session could widen `color` for
		// every theme on the deployment, retroactively, with one request.
		s.writeErr(w, http.StatusBadRequest, "bad_name", i18n.T(keyKindBadName, locale))
		return
	}
	var in kindInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyKindBadPattern, locale))
		return
	}
	ctx := r.Context()
	k, err := s.store.ThemeKinds().Get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		// Writing a kind that does not exist yet is how an operator adds one
		// nobody proposed. It starts unapproved like everything else — the
		// approve step is a separate decision even when the same person makes
		// both, because the screen then shows one shape rather than two.
		k = &domain.ThemeKind{Name: name}
	} else if err != nil {
		s.log.Error("theme kind get", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyKindInternal, locale))
		return
	}
	if in.Pattern != nil {
		// ⚠️ The +1 is load-bearing. cleanManifestText TRUNCATES, which is right
		// for prose and wrong for executable text: a regex cut to length is a
		// different regex, and some truncations still compile and mean something
		// else — so the operator would read and approve one pattern while
		// another validated every write. Cutting at limit+1 leaves anything
		// legal untouched and anything illegal one character too long, which
		// CompileCheck refuses. Bounding a pattern means saying no.
		p := cleanManifestText(*in.Pattern, theme.MaxStoredPattern+1)
		if _, cerr := theme.CompileCheck(p); cerr != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_pattern", i18n.T(keyKindBadPattern, locale)+cerr.Error())
			return
		}
		// ⚠️ An operator changing the pattern does NOT self-revoke: they are the
		// approver, and revoking their own edit would make the field
		// uneditable-in-practice. The automatic revocation is for text that
		// arrived from a FRONTEND — see recordProposedKinds.
		k.Pattern = p
	}
	if in.Description != nil {
		k.Description = cleanManifestText(*in.Description, domain.MaxKindDescription)
	}
	if in.Approved != nil {
		k.Approved = *in.Approved
	}
	if k.Pattern == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_pattern", i18n.T(keyKindBadPattern, locale))
		return
	}
	if err := s.store.ThemeKinds().Upsert(ctx, *k); err != nil {
		s.log.Error("theme kind put", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyKindInternal, locale))
		return
	}
	if in.Approved != nil {
		s.log.Warn("a theme kind's approval changed",
			"kind", name, "approved", k.Approved, "pattern", k.Pattern)
	}
	// ⚠️ The reload IS the change. Without it the row is edited and the running
	// registry still validates against whatever it loaded at boot — the console
	// reports success and the deployment disagrees.
	if err := s.ReloadThemeKinds(ctx); err != nil {
		s.log.Error("theme kind reload", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyKindInternal, locale))
		return
	}
	s.writeJSON(w, http.StatusOK, k)
}

// DELETE /api/admin/theme-kinds/{name}
//
// ⚠️ Deleting an APPROVED kind stops every token declaring it from validating.
// Not refused — an operator who needs a bad pattern gone needs it gone — but the
// console says what it costs, and revoking (approved=false) is offered first
// because it keeps the row and the attribution for when they want it back.
func (s *Server) handleAdminKindDelete(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyKindDegraded, locale))
		return
	}
	name := r.PathValue("name")
	ctx := r.Context()
	if _, err := s.store.ThemeKinds().Get(ctx, name); errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyKindNotFound, locale))
		return
	}
	if err := s.store.ThemeKinds().Delete(ctx, name); err != nil {
		s.log.Error("theme kind delete", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyKindInternal, locale))
		return
	}
	s.log.Warn("a theme kind was deleted", "kind", name)
	if err := s.ReloadThemeKinds(ctx); err != nil {
		s.log.Error("theme kind reload", "err", err)
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
