package i18n

import "testing"

func TestPickExactAndRegional(t *testing.T) {
	full := Text{"zh-CN": "开心", "en-US": "Happy"}
	cases := map[string]string{
		"zh-CN": "开心",
		"en-US": "Happy",
		// A region we do not carry falls back within its own language before it
		// falls back to another one — a zh-TW reader is better served by
		// simplified Chinese than by English.
		"zh-TW": "开心",
		"en-GB": "Happy",
		"zh":    "开心",
		// An unrecognised language lands on Default (en-US), which is where
		// Resolve would have sent the request anyway.
		"fr": "Happy",
		"":   "Happy",
	}
	for locale, want := range cases {
		if got := Pick(full, locale); got != want {
			t.Errorf("Pick(%q) = %q, want %q", locale, got, want)
		}
	}
}

// A bare language key serves every region of that language.
func TestPickBareLanguageKey(t *testing.T) {
	t2 := Text{"zh": "中文", "en": "English"}
	for locale, want := range map[string]string{
		"zh-CN": "中文", "zh-TW": "中文", "en-US": "English", "de": "English",
	} {
		if got := Pick(t2, locale); got != want {
			t.Errorf("Pick(%q) = %q, want %q", locale, got, want)
		}
	}
}

// A half-translated entry shows the user the language it does have rather than
// a blank — a missing translation is a content gap, not a rendering failure.
func TestPickFallsBackToAnything(t *testing.T) {
	only := Text{"zh-CN": "只有中文"}
	if got := Pick(only, "en-US"); got != "只有中文" {
		t.Errorf("Pick = %q, want the one entry that exists", got)
	}
	if got := Pick(nil, "en-US"); got != "" {
		t.Errorf("Pick(nil) = %q, want empty", got)
	}
	if got := Pick(Text{"zh-CN": ""}, "zh-CN"); got != "" {
		t.Errorf("an empty entry is a missing one, got %q", got)
	}
}

// The last-resort pick must not depend on Go's map iteration order, or the same
// gap would render differently between two requests.
func TestPickIsDeterministic(t *testing.T) {
	many := Text{"de": "de", "fr": "fr", "ja": "ja", "ko": "ko", "pt": "pt"}
	first := Pick(many, "es")
	for i := 0; i < 50; i++ {
		if got := Pick(many, "es"); got != first {
			t.Fatalf("iteration %d gave %q, first gave %q", i, got, first)
		}
	}
}

func TestMissing(t *testing.T) {
	if got := Missing(Text{"zh-CN": "有", "en-US": "have"}); len(got) != 0 {
		t.Errorf("complete entry reported missing: %v", got)
	}
	if got := Missing(Text{"zh-CN": "只有中文"}); len(got) != 1 || got[0] != "en-US" {
		t.Errorf("Missing = %v, want [en-US]", got)
	}
	// A bare language key covers its regions, so this is complete.
	if got := Missing(Text{"zh": "中", "en": "en"}); len(got) != 0 {
		t.Errorf("bare language keys should satisfy their regions, got %v", got)
	}
	if got := Missing(nil); len(got) != len(Embedded) {
		t.Errorf("Missing(nil) = %v, want every supported locale", got)
	}
}
