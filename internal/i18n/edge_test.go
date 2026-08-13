package i18n

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tf marks the call sites whose translations carry verbs. The fmt behaviour
// for a mismatch is locked here: a missing verb prints a %! marker rather
// than panicking, and a missing argument does the same — neither may take
// the process down.
func TestTfVerbs(t *testing.T) {
	c := NewCatalog()
	c.Register("n", Text{"zh-CN": "你有 %d 小时", "en-US": "you have %d hours"})
	if got := c.Tf("n", "zh-CN", 3); got != "你有 3 小时" {
		t.Errorf("Tf = %q", got)
	}
	if got := c.Tf("n", "zh-CN"); !strings.Contains(got, "%!d(MISSING)") {
		t.Errorf("a missing argument must render a %%! marker, got %q", got)
	}
	// A translation that dropped the verb is a translator bug the runtime
	// survives (with the marker) rather than a panic.
	c.Register("broken", Text{"zh-CN": "no verb here", "en-US": "no verb here"})
	if got := c.Tf("broken", "zh-CN", 3); !strings.Contains(got, "%!") {
		t.Errorf("extra args to a verb-less text must render a marker, got %q", got)
	}
}

func TestLoadDirRejectsNonStringValues(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zh-CN.json"), []byte(`{"k": 42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewCatalog().LoadDir(dir); err == nil {
		t.Error("a non-string value must fail the whole file — a number where text goes is a broken pack, not a stringifiable one")
	}
	// A bad locale tag in the FILENAME is refused too.
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "!!bad!!.json"), []byte(`{"k": "v"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewCatalog().LoadDir(dir2); err == nil {
		t.Error("an unusable locale tag in the filename must fail")
	}
}

func TestSetOverridesDropsIllegalLocales(t *testing.T) {
	// Locale keys that do not canonicalise are silently dropped — the table
	// is the authority and a row it could not have written is ignored, not
	// turned into an unmatchable entry.
	c := NewCatalog()
	c.Register("k", Text{"zh-CN": "你好"})
	c.SetOverrides(map[string]Text{"k": {"!!bad!!": "x", "zh-CN": "覆盖"}})
	if got := c.T("k", "zh-CN"); got != "覆盖" {
		t.Errorf("the canonical locale override must win, got %q", got)
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"zh-CN,zh;q=0.9,en;q=0.8", "zh-CN"},
		{"en-US;q=0.5, zh-CN;q=0.9", "en-US"}, // q-values are stripped, FIRST position wins — locked behaviour
		{"*", ""},
		{"zh", "zh-CN"},
		{"fr-FR", ""},
	}
	for _, tc := range cases {
		if got := FromAcceptLanguage(tc.in); got != tc.want {
			t.Errorf("FromAcceptLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOtherWithForeignCurrent(t *testing.T) {
	p := Pair{Primary: "zh-CN", Secondary: "en-US"}
	if got := p.Other("en-US"); got != "zh-CN" {
		t.Errorf("Other(en-US) = %q", got)
	}
	if got := p.Other("zh-CN"); got != "en-US" {
		t.Errorf("Other(zh-CN) = %q", got)
	}
	// A locale that is in NEITHER slot flips to the secondary — locked as the
	// current behaviour; the switch only ever offers the secondary.
	if got := p.Other("fr-FR"); got != "en-US" {
		t.Errorf("Other(fr-FR) = %q, want the secondary", got)
	}
	// One language means no switch at all.
	p2 := Pair{Primary: "zh-CN"}
	if got := p2.Other("zh-CN"); got != "" {
		t.Errorf("a single-language pair must offer nothing, got %q", got)
	}
}
