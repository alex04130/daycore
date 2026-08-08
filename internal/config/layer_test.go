package config

import (
	"reflect"
	"testing"
)

// The gate. Adding a field to Config without classifying it fails here, which is
// the whole mechanism: "can this change without a restart" gets answered once,
// by the person adding it, at the moment they have the context to answer it.
//
// Left to the console author instead, the question gets answered forty times by
// whoever is writing that screen — and the wrong answers are the dangerous ones.
// A knob wrongly marked runtime is a setting the console changes and the process
// ignores: "I turned it off and it kept doing it".
func TestEveryConfigFieldIsClassified(t *testing.T) {
	typ := reflect.TypeOf(Config{})
	var missing []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if _, ok := SettingFor(f.Name); !ok {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d config field(s) are not classified in Settings: %v\n\n"+
			"Add each one with a Layer:\n"+
			"  LayerBoot    the process built something out of it (a socket, a handle,\n"+
			"               a parsed file, a signing key) — a new value cannot change\n"+
			"               the thing already built. Also every secret.\n"+
			"  LayerRuntime read fresh at each use; a new value takes effect next read.\n\n"+
			"If the honest answer is \"runtime by nature but not hot yet\", say that in Why\n"+
			"rather than marking it boot — the console needs to know which it is.",
			len(missing), missing)
	}

	// And the reverse: a classification for a field that no longer exists is a
	// stale entry the console would render as a knob that does nothing.
	for _, s := range Settings {
		if _, ok := typ.FieldByName(s.Field); !ok {
			t.Errorf("Settings classifies %q (%s) but Config has no such field", s.Field, s.Env)
		}
	}
}

// Every secret is boot-layer, and nothing that is boot-layer is editable. These
// are two different promises and both matter: one is about restarts, the other
// is about a console that can display a signing key having already lost it.
func TestSecretsAreNeverEditable(t *testing.T) {
	for _, s := range Settings {
		if s.Secret && s.Layer != LayerBoot {
			t.Errorf("%s is a secret but classified %s — a secret is never hot-editable", s.Env, s.Layer)
		}
		if s.Secret && Editable(s.Field) {
			t.Errorf("%s is editable and is a secret", s.Env)
		}
		if s.Layer == LayerBoot && Editable(s.Field) {
			t.Errorf("%s is boot-layer and reports as editable", s.Env)
		}
	}
	// The four that must never be reachable from a web console, named
	// explicitly so that a future refactor of Settings cannot quietly drop one.
	for _, field := range []string{"JWTSecret", "CookieSecret", "Pepper", "AdminToken", "DBDSN"} {
		s, ok := SettingFor(field)
		if !ok || !s.Secret {
			t.Errorf("%s is not marked secret", field)
		}
		if Editable(field) {
			t.Errorf("%s is editable from the console", field)
		}
	}
}

// A reason is required exactly where the answer is not self-evident. A knob
// whose layer is surprising and unexplained is one somebody will "fix".
func TestSurprisingClassificationsCarryAReason(t *testing.T) {
	// Anything read per request but classified boot is a deliberate security
	// decision, and the reason is what stops it being read as an oversight.
	for _, field := range []string{"AllowedOrigins", "TrustProxyHeaders", "SecureCookies", "CookieSameSite", "JWTTTL"} {
		s, _ := SettingFor(field)
		if s.Why == "" {
			t.Errorf("%s is boot-layer for a non-obvious reason and says nothing about why", field)
		}
	}
}
