package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func seeded() *Catalog {
	c := NewCatalog()
	c.Register("greet", Text{"zh-CN": "你好", "en-US": "Hello"})
	c.Register("bye", Text{"zh-CN": "再见", "en-US": "Bye"})
	return c
}

func TestLayerPrecedence(t *testing.T) {
	c := seeded()
	c.SetOverrides(map[string]Text{"greet": {"en-US": "Hi there"}})
	if got := c.T("greet", "en-US"); got != "Hi there" {
		t.Errorf("database layer should win, got %q", got)
	}
	// Overriding one locale must not disturb the others.
	if got := c.T("greet", "zh-CN"); got != "你好" {
		t.Errorf("zh-CN = %q, want the embedded text", got)
	}
	if got := c.T("bye", "en-US"); got != "Bye" {
		t.Errorf("an untouched key = %q", got)
	}
}

// Resolution runs per locale, not per layer. A half-finished override must not
// hide a complete translation underneath it: asking for English when the
// database only has French should reach the embedded English, not the French.
func TestPartialOverrideDoesNotHideLowerLayers(t *testing.T) {
	c := seeded()
	c.SetOverrides(map[string]Text{"greet": {"fr-FR": "Bonjour"}})
	if got := c.T("greet", "en-US"); got != "Hello" {
		t.Errorf("got %q, want the embedded English rather than the French override", got)
	}
	if got := c.T("greet", "fr-FR"); got != "Bonjour" {
		t.Errorf("the French override should still be reachable, got %q", got)
	}
}

// A missing message has to look missing. A blank string reads as a layout bug
// and sends whoever finds it looking in the wrong place.
func TestUnknownKeyEchoesItself(t *testing.T) {
	if got := seeded().T("nope.not.here", "en-US"); got != "nope.not.here" {
		t.Errorf("got %q, want the key echoed back", got)
	}
}

// Two packages fighting over one key produce text that depends on link order.
func TestDuplicateKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering a key twice should panic")
		}
	}()
	c := seeded()
	c.Register("greet", Text{"en-US": "again"})
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ja-JP.json", `{"greet":"こんにちは"}`)
	write("en-US.json", `{"bye":"Later"}`)
	write("notes.txt", "ignored")

	c := seeded()
	if err := c.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := c.T("greet", "ja-JP"); got != "こんにちは" {
		t.Errorf("a language added by file should resolve, got %q", got)
	}
	if got := c.T("bye", "en-US"); got != "Later" {
		t.Errorf("a file should override embedded text, got %q", got)
	}
	// Japanese has no "bye", so it falls through — to en-US, which the file
	// also overrode.
	if got := c.T("bye", "ja-JP"); got != "Later" {
		t.Errorf("fallback = %q", got)
	}

	// A language installed purely from a file counts as available. This is the
	// whole point: adding a language must not need a rebuild.
	found := false
	for _, l := range c.Available() {
		if l == "ja-JP" {
			found = true
		}
	}
	if !found {
		t.Errorf("ja-JP missing from %v", c.Available())
	}

	// Reloading after a file is deleted must actually remove its text, or a
	// translation would outlive the file that accounted for it.
	if err := os.Remove(filepath.Join(dir, "ja-JP.json")); err != nil {
		t.Fatal(err)
	}
	if err := c.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := c.T("greet", "ja-JP"); got != "Hello" {
		t.Errorf("after deleting the pack, ja-JP should fall back, got %q", got)
	}
}

func TestLoadDirRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en-US.json"), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := seeded().LoadDir(dir); err == nil {
		t.Error("malformed JSON should be an error, not a silently empty pack")
	}

	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "not a locale.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := seeded().LoadDir(dir2); err == nil {
		t.Error("a filename that is not a locale tag should be an error")
	}

	// A missing directory is the normal case for a fresh install.
	if err := seeded().LoadDir(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("a missing locales dir should not be an error: %v", err)
	}
	if err := seeded().LoadDir(""); err != nil {
		t.Errorf("an unset locales dir should not be an error: %v", err)
	}
}

// Embedded is always available, even in a catalog nobody registered into —
// otherwise there is a bootstrap window where Normalize rejects every tag.
func TestAvailableAlwaysIncludesEmbedded(t *testing.T) {
	got := NewCatalog().Available()
	for _, want := range Embedded {
		found := false
		for _, l := range got {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s missing from %v", want, got)
		}
	}
}

func TestCoverage(t *testing.T) {
	c := seeded()
	c.SetOverrides(map[string]Text{"greet": {"ja-JP": "こんにちは"}})
	if have, total := c.Coverage("en-US"); have != total || total != 2 {
		t.Errorf("en-US coverage %d/%d, want 2/2", have, total)
	}
	if have, total := c.Coverage("ja-JP"); have != 1 || total != 2 {
		t.Errorf("ja-JP coverage %d/%d, want 1/2", have, total)
	}
}

// Export is a translator's starting point, so it must list every key — the ones
// with no text in that locale included, holding whatever the chain produced.
func TestExportIsAComleteWorklist(t *testing.T) {
	c := seeded()
	c.SetOverrides(map[string]Text{"greet": {"ja-JP": "こんにちは"}})
	out := c.Export("ja-JP")
	if len(out) != 2 {
		t.Fatalf("export has %d keys, want every key", len(out))
	}
	if out["greet"] != "こんにちは" {
		t.Errorf("greet = %q", out["greet"])
	}
	if out["bye"] != "Bye" {
		t.Errorf("an untranslated key should carry its fallback, got %q", out["bye"])
	}
}

func TestCanonical(t *testing.T) {
	for tag, want := range map[string]string{
		"zh-CN": "zh-CN", "zh_cn": "zh-CN", "ZH-cn": "zh-CN",
		"en": "en", "EN": "en", "ja-JP": "ja-JP",
		"zh-Hans-CN": "zh-Hans-CN", "es-419": "es-419",
		// Canonical does not check installation — a pack for a language nobody
		// has shipped yet still has to be loadable.
		"sw-KE": "sw-KE",
		"":      "", "  ": "", "x": "", "toolongalang": "", "en-USA": "", "12": "",
	} {
		if got := Canonical(tag); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", tag, got, want)
		}
	}
}
