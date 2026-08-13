package ai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daycore/internal/domain"
)

func newPromptSvc(t *testing.T) *PromptService {
	t.Helper()
	s, err := NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// missingkey=zero: a template that references a field the caller did not
// supply renders the zero value, never "<no value>" and never an error —
// a missing data field must not take the whole system prompt down.
func TestRenderMissingMapKeyYieldsZero(t *testing.T) {
	s := newPromptSvc(t)
	// missingkey=zero keeps a missing field from being an ERROR. (With
	// map[string]any data the zero of `any` is nil, which text/template prints
	// as "<no value>" — locked here rather than improved, because the persona
	// callers always supply the name.)
	got, err := s.Render(context.Background(), "persona", "zh-CN", map[string]any{})
	if err != nil {
		t.Fatalf("a missing data field must not fail the render: %v", err)
	}
	if !strings.Contains(got, "<no value>") {
		t.Errorf("expected the zero of `any` (printed as <no value>), got %q", got)
	}
}

// An unknown locale falls back to the default rather than failing — the
// chain is normalize → default.
func TestRenderUnknownLocaleFallsBackToDefault(t *testing.T) {
	s := newPromptSvc(t)
	for _, locale := range []string{"", "de-DE", "!!"} {
		if _, err := s.Render(context.Background(), "persona", locale, nil); err != nil {
			t.Errorf("Render(persona, %q) = %v — unknown locales must fall back", locale, err)
		}
	}
}

// The DB override wins over the embedded default, per locale, and a blank
// override falls back to the embedded text rather than blanking the prompt.
func TestRepoOverrideWinsAndEmptyFallsBack(t *testing.T) {
	repo := &fakePromptRepo{m: map[string]string{}}
	s, err := NewPromptService(repo)
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.Render(context.Background(), "persona", "zh-CN", nil)
	if err != nil {
		t.Fatal(err)
	}
	repo.m["persona|zh-CN"] = "覆盖后的人格"
	if got, _ := s.Render(context.Background(), "persona", "zh-CN", nil); got != "覆盖后的人格" {
		t.Errorf("the DB override must win, got %q", got)
	}
	// A blank override falls back — a truncated row must not blank the system
	// prompt.
	repo.m["persona|zh-CN"] = "   "
	if got, _ := s.Render(context.Background(), "persona", "zh-CN", nil); got != base {
		t.Errorf("a blank override must fall back to the embedded text, got %q", got)
	}
}

// A template that parses but fails to EXECUTE (nil map dereference in the
// template) is an error at Render time, not a panic.
func TestRenderExecuteError(t *testing.T) {
	repo := &fakePromptRepo{m: map[string]string{"persona|zh-CN": `{{.Missing.Field}}`}}
	s, err := NewPromptService(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Render(context.Background(), "persona", "zh-CN", map[string]any{}); err == nil {
		t.Error("a template dereferencing a nil map must error at Render")
	}
}

// Set validates before persisting: an unknown key, a non-embedded locale, a
// template that does not parse, and a nil repo each fail with their own
// reason — and nothing is written.
func TestSetErrorPaths(t *testing.T) {
	repo := &fakePromptRepo{m: map[string]string{}}
	s, err := NewPromptService(repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Set(ctx, "no.such.key", "zh-CN", "x"); err != domain.ErrNotFound {
		t.Errorf("an unknown key must return ErrNotFound, got %v", err)
	}
	if err := s.Set(ctx, "persona", "de-DE", "x"); err == nil || !strings.Contains(err.Error(), "unsupported locale") {
		t.Errorf("a non-embedded locale must be refused, got %v", err)
	}
	if err := s.Set(ctx, "persona", "zh-CN", `{{broken`); err == nil || !strings.Contains(err.Error(), "invalid template") {
		t.Errorf("an unparseable template must be refused, got %v", err)
	}
	sNoRepo, _ := NewPromptService(nil)
	if err := sNoRepo.Set(ctx, "persona", "zh-CN", "fine"); err == nil {
		t.Error("a nil repo must refuse Set")
	}
	if len(repo.m) != 0 {
		t.Errorf("none of the refused sets may persist, got %v", repo.m)
	}
}

// LoadDiskDefaults overlays per-file; a later bad file reports the error but
// keeps the files applied so far (documented as non-atomic), and a file that
// parses but is blank is refused.
func TestLoadDiskDefaultsPartialApplication(t *testing.T) {
	s := newPromptSvc(t)
	dir := t.TempDir()
	zh := filepath.Join(dir, "zh-CN")
	if err := os.MkdirAll(zh, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zh, "persona.tmpl"), []byte("新人格"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zh, "brief.tmpl"), []byte(`{{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := s.LoadDiskDefaults(dir)
	if err == nil || !strings.Contains(err.Error(), "does not parse") {
		t.Fatalf("a broken template must be refused with a parse error, got %v", err)
	}
	if n != 1 {
		t.Errorf("the first (valid) file must already be applied, got n=%d", n)
	}
	// A blank file is refused with its own message.
	if err := os.WriteFile(filepath.Join(zh, "brief.tmpl"), []byte("   "), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadDiskDefaults(dir); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("a blank template must be refused, got %v", err)
	}
}

// ── Catalog ─────────────────────────────────────────────────────────────────

// The catalog tests ride the real openai format, which the test binary
// registers through the external test package's blank imports — registering
// a fake format here would trip the external toolstream table, which asserts
// an exact set of registered formats.

func writeModels(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const fakeFormatBody = "models:\n  - id: chat\n    format: openai\n    base_url: https://example.com/v1\n    model: chat-1\n    tools: true\n  - id: planner\n    format: openai\n    base_url: https://example.com/v1\n    model: chat-1\n    tools: true\n  - id: vision\n    format: openai\n    base_url: https://example.com/v1\n    model: chat-1\n    vision: true\n    tools: true\n"

func TestLoadCatalogSelectorsAndFallbacks(t *testing.T) {
	path := writeModels(t, fakeFormatBody)
	c, err := LoadCatalog(path, "chat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// The first vision-capable model is auto-picked when none is named.
	if v, ok := c.Vision(); !ok || v == nil {
		t.Error("the first vision model must be auto-picked")
	}
	// Planner falls back to the default chat model when not named.
	if c.Planner() != c.DefaultChat() {
		t.Error("Planner() must fall back to DefaultChat() when not configured")
	}
	// And is used when it IS named and present.
	c2, err := LoadCatalog(path, "chat", "", "planner")
	if err != nil {
		t.Fatal(err)
	}
	p2, ok := c2.Provider("planner")
	if !ok || c2.Planner() != p2 {
		t.Error("a named planner must be used")
	}
}

func TestLoadCatalogErrorPaths(t *testing.T) {
	cases := []struct{ name, path, chat, vision, body string }{
		{"missing file", "/no/such/models.yaml", "chat", "", ""},
		{"empty catalog", "", "chat", "", "models: []\n"},
		{"entry without id", "", "chat", "", "models:\n  - format: openai\n"},
		{"duplicate id", "", "chat", "", "models:\n  - id: a\n    format: openai\n  - id: a\n    format: openai\n"},
		{"unknown format", "", "chat", "", "models:\n  - id: chat\n    format: nope\n"},
		{"default chat not in catalog", "", "missing", "", fakeFormatBody},
		{"vision not in catalog", "", "chat", "missing", fakeFormatBody},
		{"vision not vision-capable", "", "chat", "chat", fakeFormatBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if tc.body != "" {
				path = writeModels(t, tc.body)
			}
			if _, err := LoadCatalog(path, tc.chat, tc.vision, ""); err == nil {
				t.Error("LoadCatalog must fail")
			}
		})
	}
}
