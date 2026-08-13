package notify

import (
	"context"
	"strings"
	"testing"
)

type fakeNotifier struct{ sent int }

func (f *fakeNotifier) Send(context.Context, Notification) error { f.sent++; return nil }

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
	mustPanic(t, "empty name", func() { Register("", func(map[string]any) (Notifier, error) { return nil, nil }) })
	mustPanic(t, "nil opener", func() { Register("nilfn", nil) })
}

func TestRegisterDuplicatePanics(t *testing.T) {
	Register("zz-test-driver", func(map[string]any) (Notifier, error) { return nil, nil })
	mustPanic(t, "duplicate", func() { Register("zz-test-driver", func(map[string]any) (Notifier, error) { return nil, nil }) })
}

func TestOpenEmptyNameIsNilNotifier(t *testing.T) {
	// "no push configured" is a supported deployment, not an error —
	// callers must tolerate a nil Notifier.
	n, err := Open("", nil)
	if n != nil || err != nil {
		t.Fatalf("Open(\"\") = (%v, %v), want (nil, nil)", n, err)
	}
}

func TestOpenUnknownDriverNamesTheRegisteredSet(t *testing.T) {
	_, err := Open("no-such-driver", nil)
	if err == nil || !strings.Contains(err.Error(), "no-such-driver") {
		t.Fatalf("unknown driver must be named in the error, got %v", err)
	}
	if !strings.Contains(err.Error(), strings.Join(Drivers(), ",")) && len(Drivers()) > 0 {
		t.Errorf("the error should list the registered drivers %v, got %v", Drivers(), err)
	}
}

func TestDriversSorted(t *testing.T) {
	got := Drivers()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Drivers() not sorted: %v", got)
		}
	}
}

func TestOpenBuildsTheDriver(t *testing.T) {
	var gotCfg map[string]any
	Register("zz-open-test", func(cfg map[string]any) (Notifier, error) {
		gotCfg = cfg
		return &fakeNotifier{}, nil
	})
	n, err := Open("zz-open-test", map[string]any{"k": "v"})
	if err != nil || n == nil {
		t.Fatalf("Open = (%v, %v)", n, err)
	}
	if gotCfg["k"] != "v" {
		t.Errorf("the opener must receive the config, got %v", gotCfg)
	}
}
