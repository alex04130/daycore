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
// fix what it lists, done. Adding a locale to i18n.Supported without this would
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
	check("precipitation format", precipFormat)
}

// The forecast digest's one localized fragment carries a %d. Losing it in
// translation would print the format verb instead of the number.
func TestPrecipFormatKeepsItsVerb(t *testing.T) {
	f := &Forecast{Location: "北京", Days: []Day{
		{Date: "2026-07-12", Text: "阴", TempMin: 22, TempMax: 31, PrecipProb: 40},
	}}
	for _, locale := range i18n.Supported {
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
