package server

import (
	"context"

	"daycore/internal/i18n"
)

// ReloadLocaleOverrides pulls the database layer of the message catalog into
// memory.
//
// Without this call the "three layers" the docs describe are two: `i18n.Catalog`
// holds a `db` map it never receives, `domain.LocaleRepository` is implemented in
// both stores and covered by the conformance suite, and nothing connects them.
// Every claim about the console editing translations, or about translations being
// shared across instances, went through here — so the wiring is the claim.
//
// The fold lives in this package rather than in `internal/i18n` on purpose:
// the catalog is deliberately storage-free (its own comment says "nothing here
// needs a database connection and the whole catalog stays testable without
// one"), so the package that owns the store owns the loading.
//
// Call it at startup and again after any console edit. It replaces the layer
// wholesale — which is why `All` must return everything, not a page: a partial
// read would silently delete translations.
func (s *Server) ReloadLocaleOverrides(ctx context.Context) error {
	rows, err := s.store.Locales().All(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]i18n.Text, len(rows))
	for _, r := range rows {
		t, ok := m[r.Key]
		if !ok {
			t = i18n.Text{}
			m[r.Key] = t
		}
		t[r.Locale] = r.Content
	}
	i18n.Std().SetOverrides(m)
	return nil
}
