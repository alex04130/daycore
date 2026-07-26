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
func Normalize(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	base := strings.ToLower(tag)
	if i := strings.IndexAny(base, "-_"); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "zh":
		return "zh-CN"
	case "en":
		return "en-US"
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

// Resolve picks the effective locale: the session's stored language when set
// and supported, else the Accept-Language negotiation, else Default.
func Resolve(sessionLang, acceptLanguage string) string {
	if l := Normalize(sessionLang); l != "" {
		return l
	}
	if l := FromAcceptLanguage(acceptLanguage); l != "" {
		return l
	}
	return Default
}
