package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"daycore/internal/adapters"
	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (providers)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/providers", s.handleAdminProvidersGet)
		mux.HandleFunc("PUT /api/admin/providers", s.handleAdminProvidersPut)
	})
}

// The console's external-capability screen.
//
// # What it can and cannot write, and why the line is there
//
//	config/providers.yaml    id, format, base_url, token_env   file only
//	provider_overrides       enabled, description, approved    this endpoint
//
// The split is F1's rule applied per field rather than per file: did the
// process already build something out of it? An HTTP client was constructed
// from base_url at startup, so a new value cannot change the thing already
// built — the same reason DB_DSN is not editable here.
//
// base_url has a second reason, and it is the one that keeps the rule from
// eroding: **it is the SSRF entrance.** A base_url editable from a web page
// lets a compromised console point this backend at a link-local metadata
// address, and the answer travels out through days[].text → the model → the
// user's chat window. Requiring shell access is the whole defence, and it costs
// nothing real — changing an adapter's address was always an operations task.
//
// Not writing the YAML also means not having a problem that is genuinely hard:
// safely rewriting a file an operator hand-edits loses their comments and
// ordering, clobbers concurrent edits, and has a read-to-write gap.
//
// # Boundary: no secret, and nothing that stands in for one
//
// adapters.AdminView reports the token's variable NAME and whether that
// variable currently holds anything. Never the value, never a prefix, never a
// length: a masked secret still says how long it is and whether it changed
// between two reads, and the console needs neither.

var (
	keyAdminProvUnknown = i18n.Reg("admin.providers.unknown", i18n.Text{
		"zh-CN": "没有 %s/%s 这个源",
		"en-US": "There is no source %s/%s",
	})
	keyAdminProvBadDesc = i18n.Reg("admin.providers.bad_description", i18n.Text{
		"zh-CN": "%s/%s 的描述要 zh-CN 和 en-US 都有 —— 只有一种语言的描述会对另一半用户凭空消失",
		"en-US": "%s/%s needs a description in both zh-CN and en-US — one language only means it vanishes for half the users",
	})
	keyAdminProvDescTooLong = i18n.Reg("admin.providers.description_too_long", i18n.Text{
		"zh-CN": "%s/%s 的描述超过 %d 字 —— 它是要进提示词的一句话，不是一段说明",
		"en-US": "%s/%s: the description exceeds %d characters — it is one sentence going into a prompt, not a page",
	})
	keyAdminProvApproveEmpty = i18n.Reg("admin.providers.approve_empty", i18n.Text{
		"zh-CN": "%s/%s 没有描述可批准 —— 批准的意思是「这段文字我读过」",
		"en-US": "%s/%s has no description to approve — approving means \"I have read this text\"",
	})
)

// GET /api/admin/providers — every source, including disabled and unhealthy
// ones, because those are usually the ones being asked about.
func (s *Server) handleAdminProvidersGet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminProviders.unauthorized")
		return
	}
	views := s.providerViews()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"providers": views,
		// The config path is reported so the operator knows which file to edit
		// for the fields this screen cannot touch. Without it the greyed-out
		// rows are a dead end.
		"configPath": s.cfg.ProvidersConfigPath,
		// Health is per process by design, so two consoles legitimately
		// disagree. Naming the instance is what makes that explainable rather
		// than a bug report.
		"instance": s.InstanceID(),
	})
}

// providerViews collects every capability's sources, sorted so the screen does
// not reshuffle between reloads.
func (s *Server) providerViews() []adapters.AdminView {
	inst := s.InstanceID()
	out := []adapters.AdminView{}
	if s.weather != nil {
		out = append(out, s.weather.Views(inst)...)
	}
	if s.search != nil {
		out = append(out, s.search.Views(inst)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

type providerPatch struct {
	Kind        string             `json:"kind"`
	ID          string             `json:"id"`
	Enabled     *bool              `json:"enabled"`
	Description *map[string]string `json:"description"`
	Approved    *bool              `json:"approved"`
}

// PUT /api/admin/providers — set the three editable fields on one or more
// sources.
//
// The whole request is validated before any of it is written, for the reason
// the config endpoint gives: there is no transaction here, and a partial apply
// leaves the operator with some of their edits in place and no way to tell
// which.
func (s *Server) handleAdminProvidersPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.adminAuthorized(r) {
		s.writeErrL(w, locale, http.StatusUnauthorized, "unauthorized", "err.adminProviders.unauthorized")
		return
	}
	if s.store == nil {
		s.writeErrL(w, locale, http.StatusServiceUnavailable, "degraded", "err.adminProviders.degraded")
		return
	}
	var body struct {
		Providers []providerPatch `json:"providers"`
	}
	if err := s.readJSON(r, &body); err != nil || len(body.Providers) == 0 {
		s.writeErrL(w, locale, http.StatusBadRequest, "bad_request", "err.adminProviders.bad_request")
		return
	}

	rows := make([]domain.ProviderOverride, 0, len(body.Providers))
	for _, p := range body.Providers {
		src := s.providerSource(p.Kind, p.ID)
		if src == nil {
			s.writeErrf(w, locale, http.StatusBadRequest, "unknown_provider", keyAdminProvUnknown, p.Kind, p.ID)
			return
		}
		row, err := s.buildOverride(src, p, locale)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		rows = append(rows, row)
	}

	for _, row := range rows {
		if err := s.store.ProviderOverrides().Set(r.Context(), row); err != nil {
			s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminProviders.internal")
			return
		}
	}
	// The row is written; now push it into the live objects. Skipping this would
	// be the failure this whole layering exists to prevent — the console reports
	// a change and the process keeps using the old value.
	if err := s.ReloadProviders(r.Context()); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminProviders.internal2")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "providers": s.providerViews()})
}

// buildOverride merges a patch onto the source's current state.
//
// A patch is sparse: a field the console did not send keeps whatever it had.
// The alternative — treating absent as "clear it" — means a console that edits
// one checkbox has to remember to resend the description, and forgetting is
// silent.
func (s *Server) buildOverride(src *adapters.Source, p providerPatch, locale string) (domain.ProviderOverride, error) {
	view := src.View("")
	row := domain.ProviderOverride{Kind: p.Kind, ID: p.ID}

	enabled := view.Enabled
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	row.Enabled = &enabled

	desc := view.Description
	if p.Description != nil {
		desc = *p.Description
	}
	if len(desc) > 0 {
		if err := validateDescription(desc); err != nil {
			if strings.Contains(err.Error(), "long") {
				return row, errors.New(i18n.Tf(keyAdminProvDescTooLong, locale, p.Kind, p.ID, adapters.MaxDescriptionRunes))
			}
			return row, errors.New(i18n.Tf(keyAdminProvBadDesc, locale, p.Kind, p.ID))
		}
		row.Description = desc
	}

	// Editing the text revokes approval unless this same request re-approves.
	//
	// The stored hash alone does NOT cover this. It catches an edit that bypassed
	// the endpoint, but a patch that changes the description recomputes the hash
	// against the NEW words — so approval would silently carry over onto text
	// nobody read, which is precisely the "gate is decorative" failure this
	// mechanism exists to prevent. A test caught it here; nothing else would
	// have, because every visible behaviour looked correct.
	//
	// Re-approving in the same request is allowed and is the normal edit: the
	// operator is looking at the words they just typed.
	approved := view.Approved
	if p.Description != nil && !sameDescription(view.Description, desc) {
		approved = false
	}
	if p.Approved != nil {
		approved = *p.Approved
	}
	if approved && len(desc) == 0 {
		// Approving nothing is not a state that means anything, and storing it
		// would make the console show a tick next to a source whose prompt text
		// is the mechanical fallback.
		return row, errors.New(i18n.Tf(keyAdminProvApproveEmpty, locale, p.Kind, p.ID))
	}
	row.Approved = approved
	// The hash is recorded against the text being approved right now — that is
	// what makes a later edit revoke the approval instead of inheriting it.
	row.DescriptionHash = adapters.DescriptionHash(desc)
	return row, nil
}

// sameDescription compares two per-locale texts.
func sameDescription(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func validateDescription(d map[string]string) error {
	for _, l := range i18n.Embedded {
		if strings.TrimSpace(d[l]) == "" {
			return errors.New("missing " + l)
		}
	}
	for _, v := range d {
		if len([]rune(v)) > adapters.MaxDescriptionRunes {
			return errors.New("too long")
		}
	}
	return nil
}

func (s *Server) providerSource(kind, id string) *adapters.Source {
	switch adapters.Kind(kind) {
	case adapters.KindWeather:
		if s.weather != nil {
			return s.weather.Source(id)
		}
	case adapters.KindSearch:
		if s.search != nil {
			return s.search.Source(id)
		}
	}
	return nil
}

// ReloadProviders re-reads the override table into the live sources.
//
// Called at boot and after every console write, the same shape as
// ReloadSettings. It updates the three editable fields IN PLACE rather than
// rebuilding: a rebuild would construct new HTTP clients and, worse, new Health
// objects — discarding everything this process has learned about which sources
// answer. Enabling a description would then silently mark a known-dead source
// healthy, and the operator would watch it rejoin the tool band for a reason
// unconnected to what they did.
func (s *Server) ReloadProviders(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	rows, err := s.store.ProviderOverrides().All(ctx)
	if err != nil {
		return err
	}
	byKey := map[string]*domain.ProviderOverride{}
	for i := range rows {
		byKey[rows[i].Kind+"/"+rows[i].ID] = &rows[i]
	}
	apply := func(kind adapters.Kind, ids []string, get func(string) *adapters.Source) {
		for _, id := range ids {
			if src := get(id); src != nil {
				src.ApplyOverride(byKey[string(kind)+"/"+id])
			}
		}
	}
	if s.weather != nil {
		apply(adapters.KindWeather, s.weather.AllIDs(), s.weather.Source)
	}
	if s.search != nil {
		apply(adapters.KindSearch, s.search.AllIDs(), s.search.Source)
	}
	return nil
}
