package ai

import (
	"io/fs"
)

// PromptFS returns the embedded prompt filesystem for use by "daycore install".
//
// This file used to carry a `//go:build !lite` tag, with a companion that
// returned nil under `-tags lite`, advertised by `daycore install` as a "smaller
// binary, no embedded files" build. It was neither.
//
// The embed directive lives in prompts.go, which has no build tag, so the
// templates shipped in a lite binary exactly as they did in a normal one — the
// measured difference was 20 KiB out of 23.5 MB (0.08%), all of it this
// accessor. What the tag DID do was make WalkPromptFS a no-op, so `daycore
// install` under it extracted zero templates and produced a deployment
// directory with no editable prompts at all. Its only observable effect was to
// break the install command.
//
// It also contradicted a documented invariant: the embedded catalog, prompts
// and boundaries are the FLOOR (docs/DATA.md, internal/ai/boundaries.go) —
// the thing that lets the binary come up and say what is wrong when there is no
// database and no files on disk. A build that removes the floor to save 20 KiB
// is not a trade this project can make.
func PromptFS() fs.FS { return promptFS }

// WalkPromptFS is a shorthand for fs.WalkDir on the embedded prompt filesystem.
func WalkPromptFS(fn fs.WalkDirFunc) error {
	return fs.WalkDir(promptFS, ".", fn)
}
