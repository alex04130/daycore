//go:build !lite

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"daycore/internal/ai"
)

func extractEmbedded(dir string, force bool) error {
	return ai.WalkPromptFS(func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}
		target := filepath.Join(dir, path)
		if !force {
			if _, err := os.Stat(target); err == nil {
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		data, err := fs.ReadFile(ai.PromptFS(), path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
