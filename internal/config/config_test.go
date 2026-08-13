package config

import (
	"strings"
	"testing"
	"time"
)

// The gates below are the ones an .env typo or an inherited shell variable can
// silently defeat. Each test pins the branch that must fire, not just the
// happy path.

func TestSetButEmptyEnvVars(t *testing.T) {
	// SECURE_COOKIES= (set but empty) in production must count as unset: the
	// production Secure default exists so a forgotten flag cannot ship cookies
	// over plaintext, and an empty assignment in an .env is a forgotten flag.
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "x")
	t.Setenv("COOKIE_SECRET", "y")
	t.Setenv("ADMIN_TOKEN", "z")
	t.Setenv("SECURE_COOKIES", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.SecureCookies {
		t.Error("SECURE_COOKIES= (set but empty) must not defeat the production Secure default")
	}

	// HOST= (set but empty) in development must not defeat the loopback
	// fail-safe: the dev server carries dev secrets and a generated admin
	// token, and binding every interface is exactly what that fail-safe exists
	// to prevent.
	t.Setenv("APP_ENV", "development")
	t.Setenv("HOST", "")
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Host != "127.0.0.1" {
		t.Errorf("HOST= (set but empty) must fall back to the loopback fail-safe, got %q", c2.Host)
	}
}

func TestAppEnvNormalized(t *testing.T) {
	t.Setenv("APP_ENV", " Production ")
	t.Setenv("JWT_SECRET", "x")
	t.Setenv("COOKIE_SECRET", "y")
	t.Setenv("ADMIN_TOKEN", "z")
	t.Setenv("SECURE_COOKIES", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "production" {
		t.Errorf("APP_ENV=%q must normalize to %q, got %q", " Production ", "production", c.Env)
	}
	// The normalization is not cosmetic: the production branch must actually
	// have been taken, or the dev-secret fallback and missing Secure default
	// would have applied to a deployment whose owner believes otherwise.
	if !c.SecureCookies {
		t.Error("a capitalised APP_ENV must still take the production Secure default")
	}
}

func TestWhitespaceOnlySecretsAreUnset(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("JWT_SECRET", "   ")
	t.Setenv("COOKIE_SECRET", "	")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.UsingDevSecrets {
		t.Error("whitespace-only JWT_SECRET/COOKIE_SECRET must be treated as unset, not as chosen secrets")
	}
	if strings.Contains(c.JWTSecret, " ") {
		t.Errorf("the whitespace value itself must not be stored, got %q", c.JWTSecret)
	}
}

func TestProductionRequiresEachSecret(t *testing.T) {
	cases := []struct {
		name, key string
	}{
		{"JWT_SECRET", "JWT_SECRET"},
		{"COOKIE_SECRET", "COOKIE_SECRET"},
		{"ADMIN_TOKEN", "ADMIN_TOKEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("JWT_SECRET", "x")
			t.Setenv("COOKIE_SECRET", "y")
			t.Setenv("ADMIN_TOKEN", "z")
			// Unset exactly the one under test: t.Setenv makes empty count as
			// set, and empty counts as unset — which is the point of the
			// earlier tests.
			t.Setenv(tc.key, "")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "required in production") {
				t.Fatalf("missing %s must fail the boot, got %v", tc.key, err)
			}
		})
	}
}

func TestSameSiteValidation(t *testing.T) {
	t.Setenv("APP_ENV", "development")

	t.Run("invalid value is refused", func(t *testing.T) {
		t.Setenv("COOKIE_SAMESITE", "bogus")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "COOKIE_SAMESITE") {
			t.Fatalf("invalid COOKIE_SAMESITE must fail the boot, got %v", err)
		}
	})
	t.Run("capital None is accepted as none", func(t *testing.T) {
		t.Setenv("COOKIE_SAMESITE", "None")
		t.Setenv("SECURE_COOKIES", "true")
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if c.CookieSameSite != "none" {
			t.Errorf("COOKIE_SAMESITE=None must normalise to none, got %q", c.CookieSameSite)
		}
	})
	t.Run("none without Secure is refused", func(t *testing.T) {
		t.Setenv("COOKIE_SAMESITE", "none")
		t.Setenv("SECURE_COOKIES", "false")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "requires SECURE_COOKIES=true") {
			t.Fatalf("none + !Secure must fail the boot, got %v", err)
		}
	})
	t.Run("production default Secure satisfies none", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("JWT_SECRET", "x")
		t.Setenv("COOKIE_SECRET", "y")
		t.Setenv("ADMIN_TOKEN", "z")
		t.Setenv("COOKIE_SAMESITE", "none")
		t.Setenv("SECURE_COOKIES", "")
		c, err := Load()
		if err != nil {
			t.Fatalf("production + none + default Secure must load, got %v", err)
		}
		if !c.SecureCookies || c.CookieSameSite != "none" {
			t.Errorf("got Secure=%v SameSite=%q", c.SecureCookies, c.CookieSameSite)
		}
	})
}

func TestNumericHelpersFallBackOnGarbage(t *testing.T) {
	t.Setenv("AI_RATE_LIMIT_PER_MIN", "abc")
	if got := getInt("AI_RATE_LIMIT_PER_MIN", 30); got != 30 {
		t.Errorf("garbage int must fall back to the default, got %d", got)
	}
	t.Setenv("AI_RATE_LIMIT_PER_MIN", " 7 ")
	if got := getInt("AI_RATE_LIMIT_PER_MIN", 30); got != 7 {
		t.Errorf("padded int must parse, got %d", got)
	}
	t.Setenv("TRUST_PROXY_HEADERS", "TRUE")
	if !getBool("TRUST_PROXY_HEADERS", false) {
		t.Error("ParseBool's accepted spellings must pass through")
	}
	t.Setenv("TRUST_PROXY_HEADERS", "notabool")
	if getBool("TRUST_PROXY_HEADERS", false) {
		t.Error("garbage bool must fall back to the default")
	}
	t.Setenv("JWT_TTL", "notaduration")
	if got := getDuration("JWT_TTL", 168*time.Hour); got != 168*time.Hour {
		t.Errorf("garbage duration must fall back to the default, got %v", got)
	}
	t.Setenv("JWT_TTL", "30m")
	if got := getDuration("JWT_TTL", 168*time.Hour); got != 30*time.Minute {
		t.Errorf("valid duration must parse, got %v", got)
	}
}

func TestRandomTokenFormat(t *testing.T) {
	a, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 {
		t.Errorf("randomToken must be 43 URL-safe base64 chars (32 bytes), got %d", len(a))
	}
	if a == b {
		t.Error("two random tokens collided — the generator is broken")
	}
	for _, r := range a {
		if !(r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			t.Errorf("randomToken must be URL-safe base64, saw %q in %q", r, a)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b ,,c,", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
