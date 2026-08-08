package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Applying runtime overrides onto a config.
//
// The environment variable is the SEED and the stored override wins — that
// ordering is the whole of F1's runtime half. What this file adds is the part
// that has to be right for the console to be safe: a key that is not a runtime
// knob is REFUSED rather than stored, so there is never a row the console
// displays and the process ignores.

// ErrNotOverridable is returned for a key that exists but may not be changed at
// runtime. Distinct from "unknown" because the two need different words in front
// of a person: one is a typo, the other is "this needs a restart".
type ErrNotOverridable struct {
	Field  string
	Layer  Layer
	Secret bool
}

func (e ErrNotOverridable) Error() string {
	if e.Secret {
		return e.Field + " is a secret and is never editable"
	}
	return e.Field + " is a boot-layer setting: the process already built something from it, so a new value cannot change that. Set the environment variable and restart."
}

// ErrUnknownSetting is a key that is not a configuration field at all.
type ErrUnknownSetting struct{ Field string }

func (e ErrUnknownSetting) Error() string { return "unknown setting " + e.Field }

// Overridable reports whether a key may be stored, and says why not when it may
// not. The console calls this before writing, and Apply calls it again while
// reading — deliberately both, because a row could predate a reclassification.
func Overridable(field string) error {
	s, ok := SettingFor(field)
	if !ok {
		return ErrUnknownSetting{Field: field}
	}
	if s.Layer != LayerRuntime || s.Secret {
		return ErrNotOverridable{Field: field, Layer: s.Layer, Secret: s.Secret}
	}
	return nil
}

// Apply returns a COPY of c with the overrides applied.
//
// A copy, never a mutation: the boot config is shared by everything that was
// constructed from it, and rewriting its fields under a live server is a data
// race with every reader. Callers publish the copy atomically instead.
//
// Bad values are reported and skipped rather than failing the whole set. One
// unparseable row — left by an older build, or by a hand-written INSERT — must
// not cost the operator every other override they set, and least of all at boot.
func (c *Config) Apply(overrides map[string]string) (*Config, []error) {
	out := *c
	var problems []error
	v := reflect.ValueOf(&out).Elem()
	for key, raw := range overrides {
		if err := Overridable(key); err != nil {
			problems = append(problems, err)
			continue
		}
		f := v.FieldByName(key)
		if !f.IsValid() || !f.CanSet() {
			problems = append(problems, ErrUnknownSetting{Field: key})
			continue
		}
		if err := assign(f, raw); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", key, err))
		}
	}
	return &out, problems
}

// assign parses a stored string into whatever the field is.
//
// The table stores strings so that it does not have to model the type system;
// this is the one place that knows a field's type, and it is the same place the
// environment parsing lives. A duration is written the way the environment
// variable is ("30s"), because the operator typing it has seen that spelling.
func assign(f reflect.Value, raw string) error {
	raw = strings.TrimSpace(raw)
	switch f.Interface().(type) {
	case time.Duration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("want a duration like 30s or 2m: %w", err)
		}
		f.SetInt(int64(d))
		return nil
	case string:
		f.SetString(raw)
		return nil
	case bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("want true or false")
		}
		f.SetBool(b)
		return nil
	case int, int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("want a whole number")
		}
		f.SetInt(n)
		return nil
	}
	// Reached by a field whose type nothing here handles — i18n.Pair is the one
	// today. Refusing is right: silently ignoring it would make the console show
	// a change that did not happen.
	return fmt.Errorf("this setting's type cannot be edited as text yet")
}

// Format renders a field's current value the way an override would be written,
// so the console can show "what it is now" and "what you would type" as one
// string.
func Format(c *Config, field string) string {
	v := reflect.ValueOf(c).Elem().FieldByName(field)
	if !v.IsValid() {
		return ""
	}
	if d, ok := v.Interface().(time.Duration); ok {
		return d.String()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	}
	return ""
}
