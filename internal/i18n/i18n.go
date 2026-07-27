// Package i18n centralizes locale handling: which locales the app ships,
// normalization of arbitrary language tags, and Accept-Language negotiation.
// Prompt templates, session language, and the frontend all share these codes.
package i18n

import "strings"

// Default is the fallback locale when nothing usable is known.
const Default = "en-US"

// Supported lists every locale the app ships prompt templates for.
var Supported = []string{"zh-CN", "en-US"}

// IsSupported reports whether locale is exactly one of Supported.
func IsSupported(locale string) bool {
	for _, l := range Supported {
		if l == locale {
			return true
		}
	}
	return false
}

// Normalize maps an arbitrary BCP-47-ish tag onto a supported locale, or ""
// when the language is not shipped. Region/script variants collapse onto the
// shipped locale of the same base language ("en-GB" → "en-US").
//
// It derives everything from Supported rather than from a switch, so adding a
// language is one entry in one list. When Supported carries two regions of the
// same language, an unlisted third region resolves to the first of them in
// Supported order — declaration order is the tiebreak, so put the one you would
// rather serve first.
func Normalize(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	for _, l := range Supported {
		if strings.EqualFold(l, tag) {
			return l
		}
	}
	base := baseOf(tag)
	for _, l := range Supported {
		if baseOf(l) == base {
			return l
		}
	}
	return ""
}

// FromAcceptLanguage returns the first supported locale in an Accept-Language
// header, honoring its order (entries are already sorted by preference in
// practice; q-values are ignored beyond stripping).
func FromAcceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		tag := part
		if i := strings.IndexByte(tag, ';'); i >= 0 {
			tag = tag[:i]
		}
		if l := Normalize(tag); l != "" {
			return l
		}
	}
	return ""
}

// Negotiation deliberately lives on Offered, not here: resolving against
// everything the binary supports would hand a user a language their deployment
// does not offer, and the resulting UI would have no way to switch back out of
// it. See Offered.Resolve.
