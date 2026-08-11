package server

import (
	"context"
	"encoding/json"
	"net/http"

	"daycore/internal/domain"
)

// Which theme a session is currently on — per frontend family.
//
// # Why it is not one value
//
// 桌面用琉璃、手机用汀 is the ordinary case, not an exotic one. One current
// theme per session would make the phone retheme the desktop every time
// somebody touched it, and the two would fight forever because neither is
// wrong. A theme also cannot cross families at all — it is a set of values for
// ONE token space (see domain.CustomTheme.FamilyID).
//
// # ⚠️ Two homes, one per family — not two homes for one value
//
//	fallback family   sessions.current_theme, the column that has always held it
//	every other       SessionPrefs.ThemeByFamily[familyID]
//
// The column stays where it is BECAUSE the current frontend reads
// `session.currentTheme` and nothing else. Moving it into the map would have
// meant migrating every session row to change nothing observable, and leaving
// the column as a mirror of the map would have meant two writers for one value.
// This way each family's answer has exactly one home; which home depends on
// which family, which is decided by a constant.
//
// # ⚠️ The write is a read-modify-write, and it can lose a race
//
// Setting one family's theme rewrites the whole preferences blob, so two
// devices switching themes in the same instant can drop one of the two writes.
// That is a real exposure and it is accepted on purpose: the consequence is
// "the theme I picked on my phone did not stick, pick it again", the same
// exposure every other preference already carries, and the alternative — a
// JSON-path write or a side table — is a new method across four backends plus
// (for the side table) an extra query on a path that currently costs nothing
// because the blob rides along on the session row.
//
// If the theme switcher ever becomes something automated rather than something
// a person taps, revisit this: a background writer can lose races at a rate a
// human never will.

// currentThemeFor reports the theme this session is on for one family.
func currentThemeFor(sess *domain.Session, prefs SessionPrefs, familyID string) string {
	if familyID == "" || familyID == domain.FallbackFamilyID {
		if sess == nil {
			return ""
		}
		return sess.CurrentTheme
	}
	if t := prefs.ThemeByFamily[familyID]; t != "" {
		return t
	}
	// A family nobody has chosen a theme for yet is on the first builtin, which
	// is what a fresh session is on too. Returning "" would make the frontend
	// render unthemed on its very first load, which reads as a broken install.
	return domain.BuiltinThemes[0]
}

// setCurrentTheme records the choice in whichever home that family uses.
func (s *Server) setCurrentTheme(ctx context.Context, sid, familyID, theme string) error {
	if familyID == "" || familyID == domain.FallbackFamilyID {
		_, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{CurrentTheme: &theme})
		return err
	}
	sess, err := s.store.Sessions().Get(ctx, sid)
	if err != nil {
		return err
	}
	var prefs SessionPrefs
	if sess.Preferences != "" {
		// A blob that will not parse is not a reason to refuse a theme switch —
		// it is a reason to write a good one over it. Refusing would leave the
		// user unable to change anything until somebody edited the database.
		_ = json.Unmarshal([]byte(sess.Preferences), &prefs)
	}
	if prefs.ThemeByFamily == nil {
		prefs.ThemeByFamily = map[string]string{}
	}
	prefs.ThemeByFamily[familyID] = theme
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	str := string(raw)
	_, err = s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{Preferences: &str})
	return err
}

// sessionWithFamilyTheme is the session as one frontend should see it.
//
// ⚠️ `currentTheme` means "the theme YOU are on", and which frontend is asking
// is what makes that a well-defined question. So the same session read with a
// different X-Frontend-Build header legitimately returns a different value, and
// a client that never sends the header sees exactly what it always saw.
func (s *Server) sessionWithFamilyTheme(r *http.Request, sess *domain.Session) any {
	fam := s.familyFor(r)
	if fam.ID == domain.FallbackFamilyID {
		return sess
	}
	prefs := s.sessionPrefs(r.Context(), sess.ID)
	view := *sess
	view.CurrentTheme = currentThemeFor(sess, prefs, fam.ID)
	return &view
}
