package domain

import "time"

// ─── Theme switch audit log ────────────────────────────────────────────────

// ThemeSwitch is one row of the theme-switch audit log (a weak mood signal).
type ThemeSwitch struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Theme     string `json:"theme"`
	// FamilyID is which frontend this switch happened on. The log is a mood
	// signal, and "switched to 深夜紫 at 23:40" means something different
	// depending on whether that was the phone or the desktop.
	FamilyID   string    `json:"familyId"`
	SwitchedAt time.Time `json:"switchedAt"`
}

// ─── Custom themes ─────────────────────────────────────────────────────────

// CustomTheme is a user-authored (or AI-generated) color theme. Variables holds
// the CSS custom properties the frontend applies (keys restricted to the design
// system's token whitelist, values validated as colors server-side).
// Session.CurrentTheme may hold either a builtin theme id or a CustomTheme.ID.
type CustomTheme struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	// FamilyID is the frontend family this theme was authored for, and the
	// scope it is listed in.
	//
	// ⚠️ A theme is a set of values for ONE token space. Showing 琉璃's theme to
	// 汀 would mean handing it variables 汀 never declared and missing the ones
	// it needs — a theme that renders as "most of the page did not change".
	// That is why the scope is the family and not the session.
	//
	// Everything written before families existed carries FallbackFamilyID,
	// which is exactly right: the built-in token space is what validated it.
	FamilyID  string            `json:"familyId"`
	Name      string            `json:"name"`
	Base      string            `json:"base,omitempty"` // builtin id it started from ("sky"…), or ""
	Dark      bool              `json:"dark"`           // dark theme: frontend picks light text handling
	Variables map[string]string `json:"variables"`      // e.g. {"--primary": "#ff6b9d"}
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// CustomThemeUpdate carries optional fields for a partial theme update.
type CustomThemeUpdate struct {
	Name      *string
	Dark      *bool
	Variables *map[string]string
}

// BuiltinThemes are the four themes shipped with the design system.
var BuiltinThemes = []string{"sky", "sunset", "night", "nature"}
