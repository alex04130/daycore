package storage

import (
	"errors"
	"strings"
	"testing"

	"daycore/internal/domain"
)

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s must panic", name)
		}
	}()
	fn()
}

func TestRegisterRejectsInvalid(t *testing.T) {
	mustPanic(t, "empty dbType", func() { Register("", func(string) (domain.Store, error) { return nil, nil }) })
	mustPanic(t, "nil opener", func() { Register("zz-nil", nil) })
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register("zz-dup", func(string) (domain.Store, error) { return nil, nil })
	mustPanic(t, "duplicate dbType", func() {
		Register("zz-dup", func(string) (domain.Store, error) { return nil, nil })
	})
}

func TestOpenUnknownTypeWrapsErrUnsupported(t *testing.T) {
	_, err := Open("no-such-engine", "whatever")
	if err == nil {
		t.Fatal("an unknown db type must fail")
	}
	if !errors.Is(err, domain.ErrUnsupportedDBType) {
		t.Errorf("the error must wrap ErrUnsupportedDBType, got %v", err)
	}
	if !strings.Contains(err.Error(), "no-such-engine") {
		t.Errorf("the asked-for type must be named in the error, got %v", err)
	}
}

func TestOpenHandsTheDSNThrough(t *testing.T) {
	var gotDSN string
	Register("zz-dsn", func(dsn string) (domain.Store, error) {
		gotDSN = dsn
		return nil, nil
	})
	if _, err := Open("zz-dsn", "the-dsn"); err != nil {
		t.Fatal(err)
	}
	if gotDSN != "the-dsn" {
		t.Errorf("the opener must receive the DSN verbatim, got %q", gotDSN)
	}
}

func TestTypesSorted(t *testing.T) {
	got := Types()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Types() not sorted: %v", got)
		}
	}
	if len(got) == 0 {
		t.Error("Types() is empty — the sqlstore/mongostore init() registrations are missing")
	}
}
