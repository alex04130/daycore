// Package i18n centralizes locale handling: which locales an installation can
// render, canonicalization of arbitrary language tags, negotiation, and the
// three-layer message catalog (database → files → embedded).
//
// The important distinction, and the one that took a correction to get right:
//
//	Embedded    the two languages compiled into the binary — a floor, not a set
//	Available() every language this installation can render, files and database
//	            included; this is what a user chooses from
//	Pair        one user's own primary + secondary, which is what their
//	            language switch toggles between
//
// Adding a language is a translation file, not a release.
package i18n

import "strings"

// Default is the last-resort locale — the one used when nothing else is known
// and the one Export starts from.
const Default = "en-US"

// Embedded lists the locales compiled into the binary. It is deliberately
// short: it only has to cover the case where there is no database and no
// locales directory, so the app can still render and explain itself.
//
// Prompt templates are held to this same list (internal/ai/prompts.go fails
// startup when one is missing), because a missing prompt is not something a
// fallback chain can paper over.
var Embedded = []string{"zh-CN", "en-US"}

// IsEmbedded reports whether locale is one of the compiled-in pair.
func IsEmbedded(locale string) bool {
	for _, l := range Embedded {
		if l == locale {
			return true
		}
	}
	return false
}

// Canonical puts an arbitrary BCP-47-ish tag into the spelling this codebase
// uses — lowercase language, uppercase region, hyphen-separated: "zh_cn" and
// "ZH-CN" both become "zh-CN". It does NOT check that the locale is installed,
// because a translation file for a language nobody has shipped yet still has to
// be loadable.
//
// Returns "" for anything that is not a plausible tag.
func Canonical(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	parts := strings.FieldsFunc(tag, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return ""
	}
	lang := strings.ToLower(parts[0])
	if len(lang) < 2 || len(lang) > 3 || !isAlpha(lang) {
		return ""
	}
	out := lang
	for _, p := range parts[1:] {
		switch {
		case len(p) == 2 && isAlpha(p): // region
			out += "-" + strings.ToUpper(p)
		case len(p) == 4 && isAlpha(p): // script
			out += "-" + strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case len(p) == 3 && isDigits(p): // numeric region
			out += "-" + p
		default:
			return ""
		}
	}
	return out
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			if r < 'A' || r > 'Z' {
				return false
			}
		}
	}
	return true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Normalize maps an arbitrary tag onto a locale this installation can actually
// render, or "" when it cannot render that language at all. A region we do not
// have collapses onto another region of the same language ("en-GB" → "en-US").
//
// It consults the live catalog rather than a compile-time list, so a language
// added by dropping in a file is immediately negotiable.
func Normalize(tag string) string { return normalizeIn(Available(), tag) }

func normalizeIn(installed []string, tag string) string {
	c := Canonical(tag)
	if c == "" {
		return ""
	}
	for _, l := range installed {
		if strings.EqualFold(l, c) {
			return l
		}
	}
	base := baseOf(c)
	for _, l := range installed {
		if baseOf(l) == base {
			return l
		}
	}
	return ""
}

// FromAcceptLanguage returns the first renderable locale in an Accept-Language
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

// Negotiation deliberately lives on Pair, not here: resolving against every
// installed language would hand a user one they never chose, and the resulting
// UI would have no way to switch back out of it. See Pair.Resolve.
