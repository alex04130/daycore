package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"daycore/internal/i18n"
)

// PROMPTS_DIR must reach the renderer, and a partial extraction must be enough.
//
// `daycore install` writes the templates to disk and puts PROMPTS_DIR in the
// generated .env; for a while nothing read it, so the installer set up an
// override the server ignored. The overlay semantics are the other half: the
// previous loader required every template to be present on disk, which would
// have made "edit one prompt" cost the maintenance of all twenty-two.
func TestLoadDiskDefaultsOverlaysOneFile(t *testing.T) {
	s, err := NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	embeddedOther, err := s.Render(context.Background(), PromptMood, i18n.Default, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, i18n.Default), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "只有这一条被换掉了"
	if err := os.WriteFile(filepath.Join(dir, i18n.Default, PromptThemeGen+".tmpl"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := s.LoadDiskDefaults(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("applied %d files, want 1 — a partial extraction must not need the rest", n)
	}
	got, err := s.Render(context.Background(), PromptThemeGen, i18n.Default, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("disk template did not win: got %q", got)
	}
	// Everything not on disk keeps the embedded text.
	if other, err := s.Render(context.Background(), PromptMood, i18n.Default, nil); err != nil || other != embeddedOther {
		t.Errorf("an unrelated prompt changed: err=%v", err)
	}
	// The other locale of the same key is untouched.
	if _, err := s.Render(context.Background(), PromptThemeGen, "en-US", nil); err != nil {
		t.Errorf("en-US of the overlaid key broke: %v", err)
	}
}

func TestLoadDiskDefaultsRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		// A truncated file is likelier than "I meant to blank the system prompt".
		"empty":           "   \n",
		"broken template": "{{if .Missing}}unclosed",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			s, err := NewPromptService(nil)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, i18n.Default), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, i18n.Default, PromptMood+".tmpl"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := s.LoadDiskDefaults(dir); err == nil {
				t.Error("accepted it — this would fail one feature at runtime with no hint that a file on disk is why")
			}
		})
	}
}

func TestLoadDiskDefaultsEmptyDirIsNoop(t *testing.T) {
	s, err := NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.LoadDiskDefaults(""); err != nil || n != 0 {
		t.Errorf("unset PROMPTS_DIR must be a no-op: n=%d err=%v", n, err)
	}
	if n, err := s.LoadDiskDefaults(t.TempDir()); err != nil || n != 0 {
		t.Errorf("an empty dir must be a no-op, not an error: n=%d err=%v", n, err)
	}
}
