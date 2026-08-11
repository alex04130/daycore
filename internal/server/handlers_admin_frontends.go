package server

import (
	"errors"
	"net/http"
	"strings"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (frontends)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/frontends", s.handleAdminFrontends)
		mux.HandleFunc("PUT /api/admin/frontends/families/{id}", s.handleAdminFamilyPut)
		mux.HandleFunc("DELETE /api/admin/frontends/families/{id}", s.handleAdminFamilyDelete)
		mux.HandleFunc("PUT /api/admin/frontends/builds/{hash}/family", s.handleAdminBuildFamily)
	})
}

// The operator's side of the frontend handshake.
//
// # Why this exists at all: two flags that nothing could set
//
// `Pinned` and `RulesAccepted` are honoured by the handshake — a pinned family
// refuses new tokens, and unapproved rules never reach the model. They shipped
// with nothing able to SET them, which is the same defect as a permission
// nothing checks, seen from the other side: a switch that exists, is read, and
// has no hand on it. Editing the row by hand is not a mechanism.
//
// # The three decisions this screen exists to let somebody make
//
//	pin a family      "this one is real". Freezes the token space, which is the
//	                  answer to the handshake being unauthenticated: anybody can
//	                  widen an unpinned family, and pinning is how that stops.
//	approve rules     let a frontend's own prompt fragment reach the model.
//	                  Until then the backend writes a mechanical one, so the
//	                  feature works and the injection surface is zero.
//	move a build      the reason familyId is overridable: a new platform's build
//	                  joins an existing family and inherits its themes.
//
// # ⚠️ Deleting a family does NOT delete its themes
//
// It refuses while any build still points at it, because a family with builds
// is one somebody is using. What it cannot check is themes — those are session
// data and there may be thousands. So the refusal covers the case an operator
// can see, and the response says plainly what deleting leaves behind.
var (
	keyFrontendNotFound = i18n.Reg("admin.frontends.not_found", i18n.Text{
		"zh-CN": "没有这个 family 或 build",
		"en-US": "No such family or build",
	})
	keyFrontendHasBuilds = i18n.Reg("admin.frontends.has_builds", i18n.Text{
		"zh-CN": "还有 build 指着这个 family，先把它们挪走或者等它们不再连接：",
		"en-US": "builds still point at this family; move them first, or wait until they stop connecting: ",
	})
	keyFrontendBadRequest = i18n.Reg("admin.frontends.bad_request", i18n.Text{
		"zh-CN": "请求格式错误",
		"en-US": "Malformed request",
	})
	keyFrontendDegraded = i18n.Reg("admin.frontends.degraded", i18n.Text{
		"zh-CN": "数据库不可用，前端 family 读不到也改不了",
		"en-US": "The database is unavailable, so frontend families cannot be read or changed",
	})
	keyFrontendInternal = i18n.Reg("admin.frontends.internal", i18n.Text{
		"zh-CN": "前端 family 读写失败",
		"en-US": "Could not read or write the frontend family",
	})
)

func (s *Server) frontendsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyFrontendDegraded, s.requestLocale(r)))
		return false
	}
	return true
}

type familyView struct {
	domain.FrontendFamily
	// Builds is how many builds currently point at this family, and BuildList
	// is which — the console shows "琉璃 有三个 build 在连" and that is the
	// question the two-layer identity exists to make answerable.
	Builds []domain.FrontendBuild `json:"builds"`
}

// GET /api/admin/frontends — every family, its token space, and its builds.
func (s *Server) handleAdminFrontends(w http.ResponseWriter, r *http.Request) {
	if !s.frontendsReady(w, r) {
		return
	}
	ctx := r.Context()
	fams, err := s.store.Frontends().ListFamilies(ctx)
	if err != nil {
		s.log.Error("frontend families", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, s.requestLocale(r)))
		return
	}
	builds, err := s.store.Frontends().ListBuilds(ctx)
	if err != nil {
		s.log.Error("frontend builds", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, s.requestLocale(r)))
		return
	}
	byFamily := map[string][]domain.FrontendBuild{}
	for _, b := range builds {
		byFamily[b.FamilyID] = append(byFamily[b.FamilyID], b)
	}
	out := make([]familyView, 0, len(fams))
	for _, f := range fams {
		v := familyView{FrontendFamily: f, Builds: byFamily[f.ID]}
		if v.Builds == nil {
			v.Builds = []domain.FrontendBuild{}
		}
		out = append(out, v)
	}
	// Builds whose family no longer exists. Shown rather than hidden: they are
	// the only trace of a family somebody deleted while something was still
	// connecting, and a console that dropped them would leave an operator
	// wondering where a frontend went.
	orphans := []domain.FrontendBuild{}
	known := map[string]bool{}
	for _, f := range fams {
		known[f.ID] = true
	}
	for _, b := range builds {
		if !known[b.FamilyID] {
			orphans = append(orphans, b)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"families": out,
		"orphans":  orphans,
		// The kinds this deployment can validate — the console shows them beside
		// the token list so an operator reading `--glass-alpha: ratio` knows what
		// a ratio is allowed to be.
		"kinds": s.themeKindViews(),
		// The fallback family, so the console can say which token space applies
		// to a frontend that has not handshaken. See themes.go.
		"fallbackFamilyId": fallbackFamilyID,
		// ⚠️ Reported rather than hardcoded in the console. The ceiling is what
		// stands between an unauthenticated handshake and unbounded rows, so the
		// screen explaining that tradeoff must not be able to state it wrongly.
		"limits": map[string]int{
			"maxFamilies":     domain.MaxFamilies,
			"maxFamilyTokens": domain.MaxFamilyTokens,
		},
	})
}

func (s *Server) themeKindViews() []map[string]string {
	all := s.themeKinds.All()
	out := make([]map[string]string, 0, len(all))
	for _, k := range all {
		out = append(out, map[string]string{
			"name": k.Name, "pattern": k.Pattern, "description": k.Description, "origin": k.Origin,
		})
	}
	return out
}

// PUT /api/admin/frontends/families/{id} — pin, approve rules, rename.
//
// ⚠️ Three-state fields (pointers), not booleans. "Do not change pinned" and
// "set pinned to false" are different requests, and a console that could only
// express the second would silently unpin a family every time somebody renamed
// one.
func (s *Server) handleAdminFamilyPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.frontendsReady(w, r) {
		return
	}
	var body struct {
		DisplayName   *string `json:"displayName"`
		Pinned        *bool   `json:"pinned"`
		RulesAccepted *bool   `json:"rulesAccepted"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyFrontendBadRequest, locale))
		return
	}
	ctx := r.Context()
	fam, err := s.store.Frontends().GetFamily(ctx, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyFrontendNotFound, locale))
		return
	}
	if err != nil {
		s.log.Error("frontend family get", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, locale))
		return
	}
	if body.DisplayName != nil {
		fam.DisplayName = cleanManifestText(*body.DisplayName, domain.MaxTokenName)
	}
	if body.Pinned != nil {
		fam.Pinned = *body.Pinned
	}
	if body.RulesAccepted != nil {
		// ⚠️ Approving means this text starts reaching the model on the next
		// theme generation. It is logged for the same reason a permission grant
		// is: it is the moment somebody decided to trust client-supplied text.
		fam.RulesAccepted = *body.RulesAccepted
		s.log.Warn("a frontend's theme rules were approved or revoked",
			"family", fam.ID, "accepted", fam.RulesAccepted)
	}
	if err := s.store.Frontends().UpsertFamily(ctx, *fam); err != nil {
		s.log.Error("frontend family put", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, locale))
		return
	}
	s.writeJSON(w, http.StatusOK, fam)
}

// DELETE /api/admin/frontends/families/{id}
func (s *Server) handleAdminFamilyDelete(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.frontendsReady(w, r) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	builds, err := s.store.Frontends().ListBuilds(ctx)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, locale))
		return
	}
	// Refused while something still points at it. This is the case an operator
	// can see; the themes it would orphan are the case they cannot, which is
	// why the console says so beside the button rather than here.
	var pointing []string
	for _, b := range builds {
		if b.FamilyID == id {
			pointing = append(pointing, b.BuildHash)
		}
	}
	if len(pointing) > 0 {
		s.writeErr(w, http.StatusConflict, "family_in_use",
			i18n.T(keyFrontendHasBuilds, locale)+strings.Join(pointing, ", "))
		return
	}
	if err := s.store.Frontends().DeleteFamily(ctx, id); err != nil {
		s.log.Error("frontend family delete", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, locale))
		return
	}
	s.log.Warn("a frontend family was deleted", "family", id)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PUT /api/admin/frontends/builds/{hash}/family — move a build.
//
// This is the whole reason familyId is overridable: a new platform's build
// joins an existing family and inherits its themes. The handshake deliberately
// does not undo it — see the repository.
func (s *Server) handleAdminBuildFamily(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.frontendsReady(w, r) {
		return
	}
	var body struct {
		FamilyID string `json:"familyId"`
	}
	if err := s.readJSON(r, &body); err != nil || !familyIDRe.MatchString(body.FamilyID) {
		s.writeErr(w, http.StatusBadRequest, "bad_request", i18n.T(keyFrontendBadRequest, locale))
		return
	}
	ctx := r.Context()
	// The target must exist. Moving a build into a family that does not is how
	// a build becomes invisible: it would vanish from every family's list and
	// appear only among the orphans, with nothing saying why.
	if _, err := s.store.Frontends().GetFamily(ctx, body.FamilyID); err != nil {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyFrontendNotFound, locale))
		return
	}
	err := s.store.Frontends().SetBuildFamily(ctx, r.PathValue("hash"), body.FamilyID)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyFrontendNotFound, locale))
		return
	}
	if err != nil {
		s.log.Error("frontend build move", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyFrontendInternal, locale))
		return
	}
	s.log.Warn("a frontend build was moved between families",
		"build", r.PathValue("hash"), "family", body.FamilyID)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "familyId": body.FamilyID})
}
