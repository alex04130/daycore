//go:build lite

package ai

import "io/fs"

// PromptFS returns nil in lite builds (no embedded filesystem).
func PromptFS() fs.FS { return nil }

// WalkPromptFS is a no-op in lite builds.
func WalkPromptFS(fn fs.WalkDirFunc) error {
	return fs.ErrNotExist
}
