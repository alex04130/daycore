package server

import (
	"errors"
	"net/http"

	"daycore/internal/config"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (config)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/config", s.handleAdminConfigGet)
		mux.HandleFunc("PUT /api/admin/config", s.handleAdminConfigPut)
	})
}

// The console's configuration screen.
//
// It shows both layers and can only write one of them, which is the entire
// point: boot settings built something at startup, so a console that offered to
// change them would be offering something the process cannot honour.
//
// # Secrets never leave
//
// Not the value, not a prefix, not a length. A masked secret still tells an
// attacker how long it is and whether it changed between two reads, and neither
// is information the console needs to do its job — it only ever needs to say
// "this is configured" or "this is not".

type adminSettingView struct {
	Env    string `json:"env"`   // the environment variable name
	Key    string `json:"key"`   // the Config field, which is what PUT takes
	Layer  string `json:"layer"` // "boot" | "runtime"
	Value  string `json:"value,omitempty"`
	Set    bool   `json:"set"`              // configured at all (the answer for a secret)
	Secret bool   `json:"secret,omitempty"` // value withheld
	// Source says where the ACTIVE value came from: "env" (the seed) or
	// "override" (a row somebody wrote). Without it the console cannot show a
	// reset control that means anything.
	Source string `json:"source,omitempty"`
	// Editable is Layer==runtime && !Secret, precomputed so the console does not
	// re-derive a rule that lives here.
	Editable bool `json:"editable"`
	// Why carries the reason a surprising classification is what it is, so the
	// operator staring at a greyed-out field can find out without reading Go.
	Why string `json:"why,omitempty"`
	// RequiresRestart marks a knob that is runtime BY NATURE but whose value was
	// baked into something at construction — the rate limiters, the model
	// catalog, the weather chain. Storing an override is accepted and correct;
	// it simply will not take effect until a restart, and saying so is the
	// difference between a console that is honest and one that lies quietly.
	RequiresRestart bool `json:"requiresRestart,omitempty"`
}

// notHotYet are the runtime knobs whose current wiring bakes the value in at
// construction. They are classified runtime because that is what they are BY
// NATURE — the console needs to know which kind of thing it is looking at — and
// listed here because today they still need a restart.
//
// The list shrinks as each one gains a setter. It is written out rather than
// derived because there is nothing to derive it from: "was this value copied
// into something at startup" is not visible from the type.
var notHotYet = map[string]bool{
	"RateLimitPerMin":     true, // rateLimiter is constructed in New
	"AuthRateLimitPerMin": true,
	"DefaultChatModel":    true, // LoadCatalog resolves these once
	"DefaultVisionModel":  true,
	"DefaultPlannerModel": true,
	"WeatherProvider":     true, // weather.New builds the chain once
}

var (
	keyAdminCfgNotEditable = i18n.Reg("admin.config.not_editable", i18n.Text{
		"zh-CN": "%s 不能在运行时改：进程启动时已经用它造出了别的东西。改环境变量再重启。",
		"en-US": "%s cannot change at runtime: the process already built something from it. Set the environment variable and restart.",
	})
	keyAdminCfgUnknown = i18n.Reg("admin.config.unknown", i18n.Text{
		"zh-CN": "没有 %s 这个配置项",
		"en-US": "There is no setting called %s",
	})
	keyAdminCfgBadValue = i18n.Reg("admin.config.bad_value", i18n.Text{
		"zh-CN": "%s 的值不对：%s",
		"en-US": "%s: %s",
	})
)

// GET /api/admin/config — every knob, both layers, with where its value came from.
func (s *Server) handleAdminConfigGet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminConfig.unauthorized")
		return
	}
	overrides := map[string]bool{}
	if s.store != nil {
		if rows, err := s.store.Settings().All(r.Context()); err == nil {
			for _, row := range rows {
				overrides[row.Key] = true
			}
		}
		// A read failure is not fatal here: the screen is still useful showing
		// the seeds, and in degraded mode this is the ONLY thing that works.
	}

	active := s.runtime()
	out := make([]adminSettingView, 0, len(config.Settings))
	for _, set := range config.Settings {
		if set.Env == "" {
			continue // derived, not a knob
		}
		v := adminSettingView{
			Env: set.Env, Key: set.Field, Layer: string(set.Layer),
			Secret: set.Secret, Why: set.Why,
			Editable:        config.Editable(set.Field),
			RequiresRestart: notHotYet[set.Field],
		}
		raw := config.Format(active, set.Field)
		v.Set = raw != ""
		if !set.Secret {
			v.Value = raw
			v.Source = "env"
			if overrides[set.Field] {
				v.Source = "override"
			}
		}
		out = append(out, v)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"settings": out})
}

// PUT /api/admin/config — set or reset runtime overrides.
//
// A null value resets a key to its environment seed. That has to be expressible
// and has to be distinct from the empty string: for a string knob, "back to the
// default" and "set it to nothing" are different requests, and a console with
// only one of them cannot undo a mistake.
func (s *Server) handleAdminConfigPut(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if !s.adminAuthorized(r) {
		s.writeErrL(w, locale, http.StatusUnauthorized, "unauthorized", "err.adminConfig.unauthorized")
		return
	}
	if s.store == nil {
		s.writeErrL(w, locale, http.StatusServiceUnavailable, "degraded", "err.adminConfig.degraded")
		return
	}
	var body struct {
		Settings map[string]*string `json:"settings"`
	}
	if err := s.readJSON(r, &body); err != nil || len(body.Settings) == 0 {
		s.writeErrL(w, locale, http.StatusBadRequest, "bad_request", "err.adminConfig.bad_request")
		return
	}

	// Validate the WHOLE request before writing any of it. A partial apply would
	// leave the operator with some of their edits in place and no way to tell
	// which — and the console has no transaction to offer them.
	for key, val := range body.Settings {
		if err := config.Overridable(key); err != nil {
			var unknown config.ErrUnknownSetting
			if errors.As(err, &unknown) {
				s.writeErrf(w, locale, http.StatusBadRequest, "unknown_setting", keyAdminCfgUnknown, key)
				return
			}
			s.writeErrf(w, locale, http.StatusBadRequest, "not_editable", keyAdminCfgNotEditable, key)
			return
		}
		if val == nil {
			continue // a reset needs no parse
		}
		if _, problems := s.cfg.Apply(map[string]string{key: *val}); len(problems) > 0 {
			s.writeErrf(w, locale, http.StatusBadRequest, "bad_value", keyAdminCfgBadValue, key, problems[0].Error())
			return
		}
	}

	for key, val := range body.Settings {
		var err error
		if val == nil {
			err = s.store.Settings().Delete(r.Context(), key)
		} else {
			err = s.store.Settings().Set(r.Context(), key, *val)
		}
		if err != nil {
			s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminConfig.internal")
			return
		}
	}
	if err := s.ReloadSettings(r.Context()); err != nil {
		// The rows are written; the snapshot is stale until the next reload or a
		// restart. Say so rather than reporting success.
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminConfig.internal2")
		return
	}

	restart := []string{}
	for key := range body.Settings {
		if notHotYet[key] {
			restart = append(restart, key)
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		// The console shows a restart banner for exactly these. Reporting it per
		// key rather than as one boolean means the operator learns WHICH of their
		// changes is waiting, which is what they will ask next.
		"requiresRestart": restart,
	})
}
