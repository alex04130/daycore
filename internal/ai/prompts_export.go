//go:build !lite

package ai

import (
	"io/fs"
)

// PromptFS returns the embedded prompt filesystem for use by "daycore install".
func PromptFS() fs.FS { return promptFS }

// WalkPromptFS is a shorthand for fs.WalkDir on the embedded prompt filesystem.
func WalkPromptFS(fn fs.WalkDirFunc) error {
	return fs.WalkDir(promptFS, ".", fn)
}
