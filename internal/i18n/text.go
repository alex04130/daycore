package i18n

import "strings"

// Text is a set of translations keyed by locale, for short user-facing strings
// that live in Go rather than in a prompt template — category names, mood
// labels, derived lock reasons.
//
// This replaces the earlier NameZH/NameEN pair. Two named fields work for
// exactly two languages: adding a third means editing every struct that has
// them and every branch that reads them, which is a schema change for what
// should be a data change. A map makes a new language a matter of adding
// entries.
//
// Keys are the same locale codes as everywhere else ("zh-CN", "en-US"). A base
// language on its own ("zh") is a valid key and matches every region of that
// language.
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

// PickFrom resolves a locale-keyed map of anything at all — a struct of date
// vocabulary, a set of format strings — using the same chain as Pick. Presence
// of the key is what counts; unlike Pick there is no notion of an empty value,
// because only the caller knows what empty means for its own type.
func PickFrom[T any](m map[string]T, locale string) (T, bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	insertionSort(keys)
	k, ok := matchKey(keys, func(string) bool { return true }, locale)
	if !ok {
		var zero T
		return zero, false
	}
	return m[k], true
}

// matchKey walks the fallback chain over a sorted key list, asking usable()
// whether a candidate counts. Sorted input plus the Supported-first sweep is
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
	for _, l := range Supported {
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

// Missing reports which of the Supported locales a Text has no entry for.
// Tests use it to keep a registry from shipping half-translated: the prompt
// templates already fail startup when a locale is missing, and registries
// should not be held to a looser standard.
//
// A locale counts as covered when the chain reaches it *within its own
// language* — a bare "zh" entry covers zh-CN. Falling through to another
// language does not count; that is the degradation this is meant to catch.
func Missing(t Text) []string {
	var out []string
	keys := sortedKeys(t)
	usable := func(k string) bool { return t[k] != "" }
	for _, l := range Supported {
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
