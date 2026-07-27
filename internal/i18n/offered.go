package i18n

import (
	"fmt"
	"strings"
)

// Offered is the pair of locales a *deployment* puts in front of its users.
//
// Supported is a capability — every locale the code has text for. Offered is a
// product decision on top of it, and the two are deliberately different things:
// shipping translations for six languages should not turn every frontend's
// language control into a six-item dropdown. Each user needs one main language
// and one they fall back on, which renders as a two-state toggle that fits in a
// header without a menu.
//
// A deployment picks which two. A Chinese-first install offers zh-CN/en-US; the
// same binary deployed elsewhere offers en-US/zh-CN, or en-US alone.
type Offered struct {
	Primary string
	// Secondary may be empty — a single-language deployment is legitimate, and
	// its frontends should hide the switch entirely rather than show a toggle
	// with one position.
	Secondary string
}

// Offer validates a deployment's pair. Both locales must be shipped, and the
// two must differ — a toggle between a language and itself is a control that
// does nothing, which is worse than no control.
//
// An empty primary falls back to Default rather than erroring: a deployment
// that never set the variable should still boot.
func Offer(primary, secondary string) (Offered, error) {
	p := strings.TrimSpace(primary)
	if p == "" {
		p = Default
	}
	np := Normalize(p)
	if np == "" {
		return Offered{}, fmt.Errorf("i18n: primary locale %q is not one of %v", primary, Supported)
	}
	s := strings.TrimSpace(secondary)
	if s == "" {
		return Offered{Primary: np}, nil
	}
	ns := Normalize(s)
	if ns == "" {
		return Offered{}, fmt.Errorf("i18n: secondary locale %q is not one of %v", secondary, Supported)
	}
	if ns == np {
		return Offered{}, fmt.Errorf("i18n: secondary locale %q is the same as primary", secondary)
	}
	return Offered{Primary: np, Secondary: ns}, nil
}

// DefaultOffer is what a deployment gets when it configures nothing.
func DefaultOffer() Offered {
	o, err := Offer(Supported[0], Supported[1])
	if err != nil {
		return Offered{Primary: Default}
	}
	return o
}

// List returns the offered locales in switch order: primary first. Frontends
// render the control straight from this, so a one-element result means "hide
// the switch", not "render a disabled one".
func (o Offered) List() []string {
	if o.Secondary == "" {
		return []string{o.Primary}
	}
	return []string{o.Primary, o.Secondary}
}

// Has reports whether a locale is one a user may choose here.
func (o Offered) Has(locale string) bool {
	return locale != "" && (locale == o.Primary || locale == o.Secondary)
}

// Other returns the locale the switch flips to, or "" when there is nothing to
// flip to. Frontends label the button with this.
func (o Offered) Other(current string) string {
	if o.Secondary == "" {
		return ""
	}
	if current == o.Secondary {
		return o.Primary
	}
	return o.Secondary
}

// Resolve picks the effective locale, clamped to what this deployment offers:
// the session's stored language, else Accept-Language negotiation, else the
// primary.
//
// Clamping rather than erroring is the point. A session can hold a locale that
// was offered when it was written and is not offered now — the deployment
// changed its pair, or the user's account moved between installs. Reading their
// settings should not fail; it should quietly land on the primary. The write
// path (PATCH /api/session) is where an unoffered locale is refused, because
// there the user is actively choosing.
func (o Offered) Resolve(sessionLang, acceptLanguage string) string {
	if l := Normalize(sessionLang); o.Has(l) {
		return l
	}
	if l := FromAcceptLanguage(acceptLanguage); o.Has(l) {
		return l
	}
	return o.Primary
}
