package server

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"daycore/internal/config"
	"daycore/internal/i18n"
	"daycore/internal/storage"
	_ "daycore/internal/storage/sqlstore"
)

// The database layer of the message catalog must actually reach the catalog.
//
// This is the test the wiring did not have: `LocaleRepository` was implemented in
// both stores and covered by the conformance suite, `Catalog` held a `db` map, and
// for a while nothing called between them — so three documents described three
// layers while the running server had two. A repository test cannot catch that,
// because the repository was never the broken part.
func TestLocaleOverridesReachTheCatalog(t *testing.T) {
	// i18n.Std() is process-global; leave it as it was found.
	t.Cleanup(func() { i18n.Std().SetOverrides(nil) })

	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/locales.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config: &config.Config{},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})

	// A key that exists in the embedded layer, so this proves precedence rather
	// than merely proving that an unknown key can be added.
	key := personaHeading
	embedded := i18n.T(key, "zh-CN")
	if embedded == "" || embedded == key {
		t.Fatalf("%q is not in the embedded catalog, so this test would prove nothing", key)
	}

	if err := store.Locales().Set(ctx, key, "zh-CN", "从数据库来的"); err != nil {
		t.Fatal(err)
	}
	if got := i18n.T(key, "zh-CN"); got != embedded {
		t.Fatalf("the override took effect before anything loaded it: %q", got)
	}

	if err := s.ReloadLocaleOverrides(ctx); err != nil {
		t.Fatal(err)
	}
	if got := i18n.T(key, "zh-CN"); got != "从数据库来的" {
		t.Errorf("database layer did not win: got %q, want %q", got, "从数据库来的")
	}
	// The other locale must be untouched — SetOverrides replaces the layer, and a
	// per-locale override must not blank out its siblings.
	if got := i18n.T(key, "en-US"); got == "" || got == key {
		t.Errorf("overriding zh-CN lost en-US: got %q", got)
	}

	// A new language, present in no other layer, must become installable — this is
	// the "adding a language is dropping in a translation, not a release" claim.
	if err := store.Locales().Set(ctx, key, "ja-JP", "データベースから"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadLocaleOverrides(ctx); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, l := range i18n.Std().Available() {
		if l == "ja-JP" {
			found = true
		}
	}
	if !found {
		t.Errorf("a DB-only locale did not show up in Available(): %v", i18n.Std().Available())
	}

	// And removing it takes the language back out.
	if _, err := store.Locales().DeleteLocale(ctx, "ja-JP"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadLocaleOverrides(ctx); err != nil {
		t.Fatal(err)
	}
	for _, l := range i18n.Std().Available() {
		if l == "ja-JP" {
			t.Errorf("uninstalling ja-JP left it in Available(): %v", i18n.Std().Available())
		}
	}
}
