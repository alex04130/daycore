package server

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("meta", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/version", s.handleHandshake)
	})
}

// The frontend handshake.
//
// # Why it is POST on the same path as the version read
//
// `GET /api/version` stays exactly as it was — anonymous clients, old builds
// and the console read it. The POST is the same question with the caller
// introducing itself, and putting it on one path means a frontend has one
// endpoint to know about rather than two whose answers must be kept consistent.
//
// # ⚠️ EVERYTHING IN THE BODY IS THIRD-PARTY DATA
//
// Third-party and multi-platform frontends are a first-class case, so a
// manifest is treated the way any external input is, even when we wrote it:
//
//	familyId      length-bounded, character-restricted (it becomes a key)
//	token names   bounded, must look like CSS custom properties
//	token kinds   must be something this deployment can VALIDATE, or the whole
//	              declaration is refused — an unknown kind means nothing checked
//	              the values that will arrive under it
//	descriptions  bounded and stripped of newlines; they reach an operator's
//	              screen and a model's prompt
//	rules         stored, NEVER used, until an operator approves it
//
// # The token space is a UNION and there is no subset check
//
// 琉璃-app declaring a token 琉璃-web does not is not a conflict: the web build
// ignores what it does not recognise, which the protocol makes its
// responsibility. Intersecting would make adding a platform DELETE tokens from
// the other one's themes.
//
// The one real conflict is the same NAME with a different KIND — `--accent` as
// a colour in one build and a length in another means the theme is meaningless
// on one of them. That is reported, not merged.
//
// # ⚠️ It is UNAUTHENTICATED, and what that costs is bounded on purpose
//
// It has to be: a frontend handshakes before it has any credential, and that is
// the premise of the whole two-layer identity — a third-party build must be
// able to introduce itself on first contact.
//
// So an anonymous caller CAN create a family and CAN widen an existing one's
// token space. Three things bound it, and they are the answer rather than an
// afterthought:
//
//	MaxFamilies    a cap. Past it, new families are refused and an operator is
//	               told to clean up or pin. Junk is bounded, not unbounded.
//	Pinned         an operator pinning a family freezes its token space. That is
//	               the switch for "this one is real now"; the console shows
//	               unpinned families as extendable by anyone.
//	the kind gate  a declared token must name a kind this deployment can
//	               validate, so the junk that fits through is junk in a shape
//	               somebody chose.
//
// What an anonymous caller CANNOT do is get any value used: `rules` is stored
// unapproved and never reaches the model, and no token value is accepted here
// at all.
//
// # What this does NOT do yet
//
// Backfill. `pendingThemeBackfill` counts the themes that would need one; the
// button that spends the AI calls is a later batch. That is honest rather than
// half-done: a theme missing a token renders with the stylesheet's own default,
// so the count is information, not a broken state.
var (
	keyHandshakeBadFamily = i18n.Reg("handshake.bad_family", i18n.Text{
		"zh-CN": "familyId 只能是字母、数字、连字符和下划线，且不超过 64 个字符",
		"en-US": "familyId must be letters, digits, hyphens and underscores, 64 characters or fewer",
	})
	keyHandshakeBadToken = i18n.Reg("handshake.bad_token", i18n.Text{
		"zh-CN": "token 名要形如 --name，且不超过 64 个字符：",
		"en-US": "a token name must look like --name and be 64 characters or fewer: ",
	})
	keyHandshakeBadKind = i18n.Reg("handshake.bad_kind", i18n.Text{
		"zh-CN": "这个部署不认识这种 kind，所以没有东西能校验它的取值。加一种 kind 是往 THEME_KINDS_DIR 丢一个 JSON 文件：",
		"en-US": "this deployment cannot validate that kind, so nothing would check the values arriving under it. Adding one is a JSON file in THEME_KINDS_DIR: ",
	})
	keyHandshakeTooManyTokens = i18n.Reg("handshake.too_many_tokens", i18n.Text{
		"zh-CN": "token 太多了",
		"en-US": "too many tokens",
	})
	keyHandshakeKindConflict = i18n.Reg("handshake.kind_conflict", i18n.Text{
		"zh-CN": "这个 token 在同一个 family 里已经是另一种 kind 了，主题会失去意义：",
		"en-US": "this token already has a different kind in this family, which would make themes meaningless: ",
	})
	keyHandshakePinned = i18n.Reg("handshake.pinned", i18n.Text{
		"zh-CN": "这个 family 已经被运维钉住，不接受新的 token。新增的是：",
		"en-US": "this family is pinned by the operator and accepts no new tokens. The new ones are: ",
	})
	keyHandshakeTooManyFamilies = i18n.Reg("handshake.too_many_families", i18n.Text{
		"zh-CN": "这个部署上的 family 已经到上限了。这个端点在任何凭据之前就可用，所以它有上限 —— 去控制台清理不再连接的，或者把要留的钉住。",
		"en-US": "This deployment is at its family limit. This endpoint is reachable before any credential exists, so it is capped — clean up the ones nothing connects with, or pin the ones you mean to keep.",
	})
	keyHandshakeTooManyKinds = i18n.Reg("handshake.too_many_kinds", i18n.Text{
		"zh-CN": "一次提议的 kind 太多了。这个端点在任何凭据之前就可用，所以它有上限。",
		"en-US": "Too many kinds proposed at once. This endpoint is reachable before any credential exists, so it is capped.",
	})
	keyHandshakeDegraded = i18n.Reg("handshake.degraded", i18n.Text{
		"zh-CN": "数据库不可用，握手记不下来。GET /api/version 仍然可用。",
		"en-US": "The database is unavailable, so a handshake cannot be recorded. GET /api/version still works.",
	})
)

// buildStaleAfter is how old a build's last-seen may be before a handshake
// rewrites it. A frontend that handshakes on every page load must not turn that
// into a write every time; "which builds are still out there" does not need to
// be exact.
const buildStaleAfter = 30 * time.Minute

type manifestToken struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

type manifestKind struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
}

type handshakeBody struct {
	FamilyID    string `json:"familyId"`
	BuildHash   string `json:"buildHash"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	MinAPI      int    `json:"minApi"`
	Theme       struct {
		Tokens []manifestToken `json:"tokens"`
		Rules  string          `json:"rules"`
		// Kinds is the THIRD TIER: validation rules this frontend needs and
		// this deployment has never heard of.
		//
		// ⚠️ Proposing is not installing. A proposed kind is stored UNAPPROVED,
		// is not merged into the registry, and validates nothing until an
		// operator reads the pattern and agrees. Tokens declaring it are left
		// out of the family in the meantime and reported back in
		// `pendingKinds`, so the frontend knows why one of its variables is
		// missing rather than discovering it as a rendering bug.
		Kinds []manifestKind `json:"kinds"`
	} `json:"theme"`
}

var familyIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var tokenNameRe = regexp.MustCompile(`^--[a-zA-Z0-9-]{1,62}$`)

// POST /api/version — introduce yourself, get the deployment's adjudication.
func (s *Server) handleHandshake(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	var body handshakeBody
	if err := s.readJSON(r, &body); err != nil {
		s.writeErrL(w, locale, http.StatusBadRequest, "bad_request", "err.handshake.bad_request")
		return
	}
	if !familyIDRe.MatchString(body.FamilyID) {
		s.writeErr(w, http.StatusBadRequest, "bad_family", i18n.T(keyHandshakeBadFamily, locale))
		return
	}
	// ⚠️ The ARRAY is bounded here, separately from MaxProposedKinds.
	//
	// That constant caps how many rows may exist; it does not cap WORK. Each
	// proposal costs a regex compile and a database read before the row cap can
	// refuse it, so a hundred thousand of them in one unauthenticated request is
	// a hundred thousand compiles and reads — the ceiling holding perfectly
	// while the request runs for a minute. A cap on rows is not a cap on cost,
	// and on an endpoint reachable before any credential exists both are needed.
	if len(body.Theme.Kinds) > domain.MaxProposedKinds {
		s.writeErr(w, http.StatusBadRequest, "too_many_kinds", i18n.T(keyHandshakeTooManyKinds, locale))
		return
	}
	if len(body.Theme.Tokens) > domain.MaxFamilyTokens {
		s.writeErr(w, http.StatusBadRequest, "too_many_tokens", i18n.T(keyHandshakeTooManyTokens, locale))
		return
	}
	out := s.versionPayload(r)
	if s.store == nil {
		// Degraded: nothing to record against. The version half is still the
		// truth, so answer it and say why the rest is missing rather than 503 —
		// a frontend that cannot handshake can still decide whether it may talk
		// to this build at all, which is the more important half.
		out["assignedFamilyId"] = body.FamilyID
		out["handshakeRecorded"] = false
		out["note"] = i18n.T(keyHandshakeDegraded, locale)
		s.writeJSON(w, http.StatusOK, out)
		return
	}

	ctx := r.Context()

	// The third tier arrives before the tokens are judged, because a token may
	// legitimately declare a kind this very request is proposing.
	pendingKinds := s.recordProposedKinds(ctx, body.FamilyID, body.Theme.Kinds)

	// Validate the DECLARATION before anything is stored. A family whose token
	// space half-applied would be worse than one that refused: the frontend
	// would think it succeeded and the missing half would surface as "unknown
	// variable" on the first theme write.
	declared := make([]domain.TokenSpec, 0, len(body.Theme.Tokens))
	deferred := []string{}
	for _, t := range body.Theme.Tokens {
		name := strings.TrimSpace(t.Name)
		if !tokenNameRe.MatchString(name) {
			s.writeErr(w, http.StatusBadRequest, "bad_token", i18n.T(keyHandshakeBadToken, locale)+name)
			return
		}
		kind := strings.TrimSpace(t.Kind)
		if !s.themeKinds.Known(kind) {
			// ⚠️ A kind this request PROPOSED is not an error — it is the third
			// tier working. The token is held back (nothing could validate its
			// values yet) and named in the response, so the frontend can say
			// "waiting for the operator" instead of rendering a variable that
			// silently never arrives.
			//
			// A kind that is neither known nor proposed is still a hard refusal:
			// unknown means nothing would check the values that follow.
			if pendingKinds[kind] {
				deferred = append(deferred, name)
				continue
			}
			s.writeErr(w, http.StatusBadRequest, "unknown_kind", i18n.T(keyHandshakeBadKind, locale)+kind)
			return
		}
		declared = append(declared, domain.TokenSpec{
			Name: name, Kind: kind, Description: cleanManifestText(t.Description, domain.MaxTokenDescription),
		})
	}

	fam, err := s.store.Frontends().GetFamily(ctx, body.FamilyID)
	if err != nil && err != domain.ErrNotFound {
		s.log.Error("handshake family", "err", err)
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.handshake.internal")
		return
	}
	if fam == nil {
		// A new family from an unauthenticated caller. Capped, because this
		// endpoint has to be reachable before any credential exists — see the
		// note at the top of this file.
		existing, lerr := s.store.Frontends().ListFamilies(ctx)
		if lerr != nil {
			s.log.Error("handshake list families", "err", lerr)
			s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.handshake.internal")
			return
		}
		if len(existing) >= domain.MaxFamilies {
			s.writeErr(w, http.StatusConflict, "too_many_families", i18n.T(keyHandshakeTooManyFamilies, locale))
			return
		}
		fam = &domain.FrontendFamily{ID: body.FamilyID, DisplayName: body.DisplayName, Tokens: []domain.TokenSpec{}}
	}

	newTokens, conflicts := mergeTokenSpace(fam, declared)
	if len(conflicts) > 0 {
		// ⚠️ Not merged and not silently preferred. The same name with two kinds
		// means one of the two builds renders a theme that is meaningless, and
		// picking a winner here would decide which one without telling anybody.
		s.writeErr(w, http.StatusConflict, "kind_conflict",
			i18n.T(keyHandshakeKindConflict, locale)+strings.Join(conflicts, ", "))
		return
	}
	if fam.Pinned && len(newTokens) > 0 {
		s.writeErr(w, http.StatusConflict, "family_pinned",
			i18n.T(keyHandshakePinned, locale)+strings.Join(newTokens, ", "))
		return
	}
	// ⚠️ Rules are STORED and not used. rulesAccepted stays whatever the
	// operator set; a new build cannot approve its own prompt fragment by
	// sending it again.
	if rules := cleanManifestText(body.Theme.Rules, domain.MaxThemeRules); rules != "" && rules != fam.Rules {
		fam.Rules = rules
		fam.RulesAccepted = false
	}
	if fam.DisplayName == "" {
		fam.DisplayName = cleanManifestText(body.DisplayName, domain.MaxTokenName)
	}
	if err := s.store.Frontends().UpsertFamily(ctx, *fam); err != nil {
		s.log.Error("handshake upsert family", "err", err)
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.handshake.internal")
		return
	}

	assigned := fam.ID
	if body.BuildHash != "" {
		b := domain.FrontendBuild{
			BuildHash:   cleanManifestText(body.BuildHash, 128),
			FamilyID:    fam.ID,
			DisplayName: cleanManifestText(body.DisplayName, domain.MaxTokenName),
			Version:     cleanManifestText(body.Version, 64),
			MinAPI:      body.MinAPI,
		}
		if err := s.store.Frontends().SeenBuild(ctx, b, time.Now().Add(-buildStaleAfter)); err != nil {
			// Best-effort: a build sighting that failed to record is not a reason
			// to refuse the handshake the frontend needs an answer to.
			s.log.Warn("could not record the frontend build", "build", b.BuildHash, "err", err)
		} else if existing, gerr := s.buildFamily(ctx, b.BuildHash); gerr == nil && existing != "" {
			// The OPERATOR's assignment wins over what the build declared. That
			// is the whole reason familyId is overridable — moving a build into
			// an existing family is how a new platform inherits its themes.
			assigned = existing
		}
	}

	out["assignedFamilyId"] = assigned
	out["rulesAccepted"] = fam.RulesAccepted
	out["newTokens"] = newTokens
	out["handshakeRecorded"] = true
	out["pendingThemeBackfill"] = s.countThemesMissingTokens(ctx, newTokens)
	// The third tier's answer: which kinds are waiting on a person, and which
	// tokens are held back until then.
	out["pendingKinds"] = sortedKeys(pendingKinds)
	out["deferredTokens"] = deferred
	s.writeJSON(w, http.StatusOK, out)
}

// buildFamily reports which family a build is currently assigned to.
func (s *Server) buildFamily(ctx context.Context, hash string) (string, error) {
	builds, err := s.store.Frontends().ListBuilds(ctx)
	if err != nil {
		return "", err
	}
	for _, b := range builds {
		if b.BuildHash == hash {
			return b.FamilyID, nil
		}
	}
	return "", nil
}

// mergeTokenSpace folds a declaration into the family's union.
//
// Returns the names that are NEW and the names that CONFLICT. A conflict is the
// same name with a different kind; everything else is a union.
func mergeTokenSpace(fam *domain.FrontendFamily, declared []domain.TokenSpec) (added, conflicts []string) {
	added, conflicts = []string{}, []string{}
	for _, t := range declared {
		existing, ok := fam.TokenByName(t.Name)
		if !ok {
			fam.Tokens = append(fam.Tokens, t)
			added = append(added, t.Name)
			continue
		}
		if existing.Kind != t.Kind {
			conflicts = append(conflicts, t.Name+" ("+existing.Kind+" vs "+t.Kind+")")
			continue
		}
		// Same name, same kind: keep the description the family already has.
		// A later build cannot rewrite what an operator is reading beside a
		// token they already approved.
	}
	sort.Strings(added)
	sort.Slice(fam.Tokens, func(i, j int) bool { return fam.Tokens[i].Name < fam.Tokens[j].Name })
	return added, conflicts
}

// countThemesMissingTokens counts USER-GENERATED themes that lack any of the
// new tokens.
//
// ⚠️ Built-in themes are deliberately not counted, and it is not an oversight:
// their values come from the frontend author and arrive with the manifest, so
// the union completes them for free. Counting both would show an operator a
// price that is mostly imaginary, and they would put off a merge that is
// actually cheap in order to avoid it.
func (s *Server) countThemesMissingTokens(ctx context.Context, newTokens []string) int {
	if len(newTokens) == 0 || s.store == nil {
		return 0
	}
	n, err := s.store.Browser().CountRows(ctx, "custom_themes")
	if err != nil {
		return 0
	}
	// Every stored custom theme predates these tokens by definition — they were
	// new to the family a moment ago. An exact per-theme check would need a
	// deployment-wide theme read this endpoint has no business doing on a
	// handshake.
	return int(n)
}

// cleanManifestText bounds a free-text field from a manifest and takes the
// newlines out.
//
// Newlines because these strings reach an operator's screen and a model's
// prompt: a description carrying "\n\nIgnore the above" is the cheapest prompt
// injection there is, and it costs nothing to make it impossible.
func cleanManifestText(s string, max int) string {
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(strings.TrimSpace(s))
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return strings.TrimSpace(s)
}
