package i18n

import "strings"

// Text is one message in every language that has it — the value type of the
// catalog, and the shape a package hands to Register.
//
// Keys are the same locale codes as everywhere else ("zh-CN", "en-US"). A base
// language on its own ("zh") is a valid key and matches every region of that
// language.
//
// Pick is the resolution chain. Almost nothing should call it directly: go
// through Catalog.T so that text installed from a file or the console wins over
// what is compiled in. Reading a Text literal straight is how a string quietly
// becomes untranslatable.
type Text map[string]string

// Pick returns the best translation for locale, walking a fallback chain rather
// than returning empty:
//
//  1. exact match — "zh-CN"
//  2. the base language — "zh" for a "zh-TW" request, or any "zh-*" entry
//  3. Default (en-US), then its base language
//  4. any entry at all
//
// Step 4 is the one that matters in practice: a half-translated registry should
// show the user *something* in a language they may not read, not a blank space
// where a mood name should be. A missing translation is a content gap, not a
// rendering failure.
func Pick(t Text, locale string) string {
	// An empty string is a missing entry, not a translation into silence.
	k, ok := matchKey(sortedKeys(t), func(k string) bool { return t[k] != "" }, locale)
	if !ok {
		return ""
	}
	return t[k]
}

// matchKey walks the fallback chain over a sorted key list, asking usable()
// whether a candidate counts. Sorted input plus the Embedded-first sweep is
// what keeps the last resort from depending on Go's map iteration order — the
// same gap must render the same way on every request.
func matchKey(keys []string, usable func(string) bool, locale string) (string, bool) {
	if len(keys) == 0 {
		return "", false
	}
	has := func(k string) bool {
		for _, x := range keys {
			if x == k {
				return usable(k)
			}
		}
		return false
	}
	if has(locale) {
		return locale, true
	}
	if k, ok := matchBase(keys, usable, baseOf(locale)); ok {
		return k, true
	}
	if has(Default) {
		return Default, true
	}
	if k, ok := matchBase(keys, usable, baseOf(Default)); ok {
		return k, true
	}
	for _, l := range Embedded {
		if has(l) {
			return l, true
		}
	}
	for _, k := range keys {
		if usable(k) {
			return k, true
		}
	}
	return "", false
}

// matchBase finds a key whose language matches base, preferring an exact
// bare-language key over a regional one.
func matchBase(keys []string, usable func(string) bool, base string) (string, bool) {
	if base == "" {
		return "", false
	}
	for _, k := range keys {
		if k == base && usable(k) {
			return k, true
		}
	}
	for _, k := range keys {
		if baseOf(k) == base && usable(k) {
			return k, true
		}
	}
	return "", false
}

func baseOf(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		return tag[:i]
	}
	return tag
}

// sortedKeys keeps resolution deterministic without pulling in sort for a
// handful of entries — these maps have two or three keys.
func sortedKeys(t Text) []string {
	out := make([]string, 0, len(t))
	for k := range t {
		out = append(out, k)
	}
	insertionSort(out)
	return out
}

func insertionSort(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Missing reports which of the Embedded locales a Text has no entry for.
// Tests use it to keep a compiled-in registry from shipping half-translated:
// the prompt templates already fail startup when a locale is missing, and the
// embedded floor should not be held to a looser standard.
//
// It checks Embedded rather than Available deliberately — a language installed
// from a file is allowed to be incomplete (Catalog.Coverage reports how
// incomplete), because the fallback chain covers the gaps. The two compiled-in
// languages are the thing the chain falls back *to*, so they have to be whole.
//
// A locale counts as covered when the chain reaches it *within its own
// language* — a bare "zh" entry covers zh-CN. Falling through to another
// language does not count; that is the degradation this is meant to catch.
func Missing(t Text) []string {
	var out []string
	keys := sortedKeys(t)
	usable := func(k string) bool { return t[k] != "" }
	for _, l := range Embedded {
		if usable(l) && contains(keys, l) {
			continue
		}
		if _, ok := matchBase(keys, usable, baseOf(l)); ok {
			continue
		}
		out = append(out, l)
	}
	return out
}

func contains(keys []string, k string) bool {
	for _, x := range keys {
		if x == k {
			return true
		}
	}
	return false
}
