package domain

import "time"

// ─── Theme switch audit log ────────────────────────────────────────────────

// ThemeSwitch is one row of the theme-switch audit log (a weak mood signal).
type ThemeSwitch struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	Theme      string    `json:"theme"`
	SwitchedAt time.Time `json:"switchedAt"`
}

// ─── Custom themes ─────────────────────────────────────────────────────────

// CustomTheme is a user-authored (or AI-generated) color theme. Variables holds
// the CSS custom properties the frontend applies (keys restricted to the design
// system's token whitelist, values validated as colors server-side).
// Session.CurrentTheme may hold either a builtin theme id or a CustomTheme.ID.
type CustomTheme struct {
	ID        string            `json:"id"`
	SessionID string            `json:"sessionId"`
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
