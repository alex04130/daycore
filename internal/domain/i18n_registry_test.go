package domain

import (
	"testing"

	"daycore/internal/i18n"
)

// Every user-facing string that lives in a Go registry must exist in every
// supported locale. The prompt templates already fail startup when a locale is
// missing (internal/ai/prompts.go); registries are held to the same standard,
// just at test time rather than boot time — a missing mood label degrades to
// another language instead of taking the server down with it.
//
// This is the gate that makes adding a language a bounded job: run the tests,
// fix what it lists, done. Adding a locale to i18n.Embedded without this would
// mean hunting for gaps by hand.
func TestRegistriesCoverEverySupportedLocale(t *testing.T) {
	check := func(what string, texts i18n.Text) {
		t.Helper()
		if missing := i18n.Missing(texts); len(missing) > 0 {
			t.Errorf("%s has no text for %v", what, missing)
		}
	}

	for _, k := range MoodKinds() {
		check("mood "+k.ID, k.Names)
	}
	for _, c := range MaterialCategories() {
		check("category "+c.ID, c.Names)
		check("category "+c.ID+" classifier hint", c.Hints)
	}
	for _, lvl := range []LockLevel{LockHard, LockSoft} {
		check("default lock reason "+string(lvl), defaultLockReasons[lvl])
	}
	check("precipitation format", i18n.Std().Lookup(precipFormat))
}

// Everything a registry registers has to be reachable by the key the rest of
// the system uses. A key typo is invisible until the day someone installs a
// language pack and finds one string that never changes.
func TestRegistryKeysResolve(t *testing.T) {
	for _, k := range MoodKinds() {
		if got := k.MoodName("zh-CN"); got == MoodNameKey(k.ID) {
			t.Errorf("mood %s resolves to its own key — not registered", k.ID)
		}
	}
	for _, c := range MaterialCategories() {
		if got := c.Name("zh-CN"); got == CategoryNameKey(c.ID) {
			t.Errorf("category %s name resolves to its own key", c.ID)
		}
		if got := c.Hint("zh-CN"); got == CategoryHintKey(c.ID) {
			t.Errorf("category %s hint resolves to its own key", c.ID)
		}
	}
	for _, lvl := range []LockLevel{LockHard, LockSoft} {
		if got := DefaultLockReason(lvl, "zh-CN"); got == LockReasonKey(lvl) {
			t.Errorf("lock reason %s resolves to its own key", lvl)
		}
	}
	// An unlocked block has no derived reason at all — not a key echo.
	if got := DefaultLockReason(LockNone, "zh-CN"); got != "" {
		t.Errorf("DefaultLockReason(none) = %q, want empty", got)
	}
	if got := DefaultLockReason(LockUnset, "zh-CN"); got != "" {
		t.Errorf("DefaultLockReason(unset) = %q, want empty", got)
	}
}

// The forecast digest's one localized fragment carries a %d. Losing it in
// translation would print the format verb instead of the number.
func TestPrecipFormatKeepsItsVerb(t *testing.T) {
	f := &Forecast{Location: "北京", Days: []Day{
		{Date: "2026-07-12", Text: "阴", TempMin: 22, TempMax: 31, PrecipProb: 40},
	}}
	for _, locale := range i18n.Embedded {
		got := f.Summary(locale)
		if want := "40"; !contains(got, want) {
			t.Errorf("Summary(%s) = %q, lost the probability", locale, got)
		}
		if contains(got, "%!") || contains(got, "%d") {
			t.Errorf("Summary(%s) = %q, format verb mismatch", locale, got)
		}
	}
	if got := f.Summary("zh-CN"); got != "北京: 07-12 阴 22~31°C 降水40%" {
		t.Errorf("zh digest = %q", got)
	}
	if got := f.Summary("en-US"); got != "北京: 07-12 阴 22~31°C precip 40%" {
		t.Errorf("en digest = %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
