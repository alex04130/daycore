package server

import "testing"

func TestValidateThemeVariables(t *testing.T) {
	ok := map[string]string{
		"--primary": "#f472b6",
		"--surface": "rgba(255,255,255,0.72)",
		"--bg-end":  "hsl(210, 40%, 96%)",
	}
	if err := validateThemeVariables(ok); err != nil {
		t.Fatalf("valid variables rejected: %v", err)
	}

	bad := []map[string]string{
		{},                                     // empty
		{"--nope": "#fff"},                     // unknown key
		{"--primary": "red"},                   // named colors not allowed
		{"--primary": "#fff; } body { x: 1"},   // injection attempt
		{"--primary": "url(https://evil.com)"}, // url
		{"--primary": "var(--other)"},          // var reference
		{"--primary": "linear-gradient(#fff, #000)"},
	}
	for i, vars := range bad {
		if err := validateThemeVariables(vars); err == nil {
			t.Fatalf("case %d: invalid variables accepted: %v", i, vars)
		}
	}
}

func TestSanitizeThemeVariables(t *testing.T) {
	clean, dropped := sanitizeThemeVariables(map[string]string{
		"--primary":  " #f472b6 ", // trimmed
		"--evil":     "#fff",
		"--bg-start": "expression(alert(1))",
	})
	if len(clean) != 1 || clean["--primary"] != "#f472b6" {
		t.Fatalf("clean = %v", clean)
	}
	if len(dropped) != 2 {
		t.Fatalf("dropped = %v", dropped)
	}
}

func TestBuiltinPresetsAreValid(t *testing.T) {
	for id, preset := range builtinThemePresets {
		if err := validateThemeVariables(preset.Variables); err != nil {
			t.Fatalf("builtin %s fails its own whitelist: %v", id, err)
		}
		if len(preset.Variables) != len(themeVarWhitelist) {
			t.Fatalf("builtin %s does not cover all %d tokens", id, len(themeVarWhitelist))
		}
	}
}
