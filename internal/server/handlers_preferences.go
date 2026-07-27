package server

import (
	"context"
	"encoding/json"
	"net/http"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

// sessionPrefsPatch is the request body for PATCH /api/session/preferences.
// All fields are optional — nil means "leave unchanged".
type sessionPrefsPatch struct {
	MorningBrief   *bool `json:"morningBrief"`
	EveningReview  *bool `json:"eveningReview"`
	DeadlineAlerts *bool `json:"deadlineAlerts"`
	RollingReplan  *bool `json:"rollingReplan"`
	GapSuggestions *bool `json:"gapSuggestions"`
	DoNotDisturb   *bool `json:"doNotDisturb"`
	AutoPlan       *bool `json:"autoPlan"`

	// MaterialCategories merges per-key: only the ids present in the request
	// change. Unknown ids and "note": false are rejected.
	MaterialCategories map[string]bool `json:"materialCategories"`

	// PrimaryLocale and SecondaryLocale are the user's own two languages. They
	// are validated together, because "primary" and "secondary" only mean
	// anything as a pair — setting one to the current value of the other has to
	// be caught, and it cannot be caught one field at a time.
	//
	// Send "" for secondary to drop the switch entirely.
	PrimaryLocale   *string `json:"primaryLocale"`
	SecondaryLocale *string `json:"secondaryLocale"`
}

// sessionPrefs loads a session's preferences, falling back to defaults (the
// same parse the GET/PATCH handlers use, packaged for other handlers).
func (s *Server) sessionPrefs(ctx context.Context, sid string) SessionPrefs {
	prefs := DefaultPrefs()
	if sess, err := s.store.Sessions().Get(ctx, sid); err == nil && sess.Preferences != "" {
		_ = json.Unmarshal([]byte(sess.Preferences), &prefs)
	}
	return prefs
}

// GET /api/session/preferences — return current session preferences.
func (s *Server) handleSessionGetPreferences(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	sess, err := s.store.Sessions().Get(r.Context(), sid)
	if err != nil {
		s.log.Error("get session preferences", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "获取偏好失败")
		return
	}
	prefs := DefaultPrefs()
	if sess.Preferences != "" {
		if err := json.Unmarshal([]byte(sess.Preferences), &prefs); err != nil {
			s.log.Error("parse session preferences", "err", err)
		}
	}
	s.writeJSON(w, http.StatusOK, prefs)
}

// PATCH /api/session/preferences — merge partial preferences into the session.
func (s *Server) handleSessionPreferences(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var patch sessionPrefsPatch
	if err := s.readJSON(r, &patch); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}

	ctx := r.Context()
	sess, err := s.store.Sessions().Get(ctx, sid)
	if err != nil {
		s.log.Error("get session for preferences", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "获取会话失败")
		return
	}

	// Start from existing prefs (or defaults if none stored).
	prefs := DefaultPrefs()
	if sess.Preferences != "" {
		if err := json.Unmarshal([]byte(sess.Preferences), &prefs); err != nil {
			s.log.Error("parse session preferences", "err", err)
		}
	}

	// Merge: overwrite only fields that were present in the request.
	if patch.MorningBrief != nil {
		prefs.MorningBrief = *patch.MorningBrief
	}
	if patch.EveningReview != nil {
		prefs.EveningReview = *patch.EveningReview
	}
	if patch.DeadlineAlerts != nil {
		prefs.DeadlineAlerts = *patch.DeadlineAlerts
	}
	if patch.RollingReplan != nil {
		prefs.RollingReplan = *patch.RollingReplan
	}
	if patch.GapSuggestions != nil {
		prefs.GapSuggestions = *patch.GapSuggestions
	}
	if patch.DoNotDisturb != nil {
		prefs.DoNotDisturb = *patch.DoNotDisturb
	}
	if patch.AutoPlan != nil {
		prefs.AutoPlan = *patch.AutoPlan
	}
	if patch.PrimaryLocale != nil || patch.SecondaryLocale != nil {
		primary, secondary := prefs.PrimaryLocale, prefs.SecondaryLocale
		if patch.PrimaryLocale != nil {
			primary = *patch.PrimaryLocale
		}
		if patch.SecondaryLocale != nil {
			secondary = *patch.SecondaryLocale
		}
		// Resolve unset halves against the deployment default before
		// validating: someone who has never touched this and now sets only a
		// secondary is choosing a pair with the default primary, and that pair
		// is what has to be legal.
		effective := i18n.PairOr(primary, secondary, s.defaultLocales)
		if primary != "" || secondary != "" {
			var err error
			if effective, err = i18n.NewPair(effective.Primary, effective.Secondary); err != nil {
				s.writeErr(w, http.StatusBadRequest, "unsupported_locale", err.Error())
				return
			}
		}
		prefs.PrimaryLocale, prefs.SecondaryLocale = effective.Primary, effective.Secondary

		// The language they are reading in must stay inside their new pair, or
		// they would be left on a language their switch can no longer reach.
		if !effective.Has(i18n.Normalize(sess.Language)) {
			lang := effective.Primary
			if _, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{Language: &lang}); err != nil {
				s.log.Error("realign session language to new pair", "err", err)
			}
		}
	}
	if patch.MaterialCategories != nil {
		if prefs.MaterialCategories == nil {
			prefs.MaterialCategories = map[string]bool{}
		}
		for id, on := range patch.MaterialCategories {
			if _, ok := domain.MaterialCategoryByID(id); !ok {
				s.writeErr(w, http.StatusBadRequest, "bad_category", "未知的资料类别: "+id)
				return
			}
			if id == domain.CategoryNote && !on {
				s.writeErr(w, http.StatusBadRequest, "bad_category", "通用笔记类别不可停用")
				return
			}
			prefs.MaterialCategories[id] = on
		}
	}

	raw, err := json.Marshal(prefs)
	if err != nil {
		s.log.Error("marshal session preferences", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "偏好保存失败")
		return
	}
	prefsStr := string(raw)
	if _, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{Preferences: &prefsStr}); err != nil {
		s.log.Error("update session preferences", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "偏好保存失败")
		return
	}
	s.writeJSON(w, http.StatusOK, prefs)
}
