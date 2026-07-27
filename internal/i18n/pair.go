package i18n

import (
	"fmt"
	"strings"
)

// Pair is one *user's* two languages: the one they read in, and the one they
// fall back on. The language switch on every frontend's home page toggles
// between exactly these two.
//
// Two rather than a list of everything installed, because the control has to be
// a button and not a menu — an installation with six translations should not
// give every user a six-item dropdown in their header. Two rather than one,
// because the second is what makes the switch worth having: the person reading
// Chinese who wants the English term for something, the bilingual user who
// reads faster in one and thinks in the other.
//
// Which two is the *user's* choice, made on the settings page. A deployment
// only supplies the default for someone who has not chosen yet
// (DEFAULT_PRIMARY_LOCALE / DEFAULT_SECONDARY_LOCALE) — it does not restrict
// what they may choose. Anything in Available() is fair game.
type Pair struct {
	Primary string
	// Secondary may be empty: someone who only reads one language should not
	// have a switch at all, and the frontends hide it rather than showing a
	// toggle with one position.
	Secondary string
}

// NewPair validates a user's choice. Both must be locales this installation can
// render, and the two must differ — a toggle between a language and itself is a
// control that does nothing, which is worse than no control.
//
// An empty primary falls back to Default rather than erroring, so a user who
// somehow has neither set still gets a working page.
func NewPair(primary, secondary string) (Pair, error) {
	p := strings.TrimSpace(primary)
	if p == "" {
		p = Default
	}
	np := Normalize(p)
	if np == "" {
		return Pair{}, fmt.Errorf("i18n: %q is not installed (have %v)", primary, Available())
	}
	s := strings.TrimSpace(secondary)
	if s == "" {
		return Pair{Primary: np}, nil
	}
	ns := Normalize(s)
	if ns == "" {
		return Pair{}, fmt.Errorf("i18n: %q is not installed (have %v)", secondary, Available())
	}
	if ns == np {
		return Pair{}, fmt.Errorf("i18n: secondary %q is the same language as primary", secondary)
	}
	return Pair{Primary: np, Secondary: ns}, nil
}

// PairOr builds a user's pair, falling back to the deployment default for
// whichever half they have not chosen. This is the normal read path: most users
// never open the language settings, and the ones who set only a primary should
// keep the default secondary rather than losing their switch.
func PairOr(userPrimary, userSecondary string, def Pair) Pair {
	p := Normalize(userPrimary)
	if p == "" {
		p = def.Primary
	}
	s := Normalize(userSecondary)
	if s == "" {
		s = def.Secondary
	}
	if s == p {
		// Their chosen primary happens to be the default secondary. Give them
		// the other half of the default rather than a dead switch.
		if def.Primary != p {
			s = def.Primary
		} else {
			s = ""
		}
	}
	if p == "" {
		p = Default
	}
	return Pair{Primary: p, Secondary: s}
}

// List returns the pair in switch order, primary first. Frontends render the
// control straight from this: one element means hide the switch, not disable
// it.
func (p Pair) List() []string {
	if p.Secondary == "" {
		return []string{p.Primary}
	}
	return []string{p.Primary, p.Secondary}
}

// Has reports whether a locale is one of this user's two.
func (p Pair) Has(locale string) bool {
	return locale != "" && (locale == p.Primary || locale == p.Secondary)
}

// Other returns the locale the switch flips to, or "" when there is nothing to
// flip to. Frontends label the button with it.
func (p Pair) Other(current string) string {
	if p.Secondary == "" {
		return ""
	}
	if current == p.Secondary {
		return p.Primary
	}
	return p.Secondary
}

// Resolve picks the effective locale for a request: the language the user is
// currently reading in when it is one of their two, else Accept-Language when
// that matches one of their two, else their primary.
//
// Clamping rather than erroring is the point. A stored "currently reading"
// value can go stale — they changed their pair, an installed language was
// removed. Reading their settings page must not fail; it should quietly land on
// their primary. The write path is where a bad choice is refused, because there
// the user is actively choosing.
func (p Pair) Resolve(current, acceptLanguage string) string {
	if l := Normalize(current); p.Has(l) {
		return l
	}
	if l := FromAcceptLanguage(acceptLanguage); p.Has(l) {
		return l
	}
	return p.Primary
}
