package ai

import _ "embed"

// The model catalog seed.
//
// # Why it is embedded rather than shipped beside the binary
//
// LoadCatalog is the one loader in the boot path with no tolerance for a
// missing file: no models means no AI, and starting anyway would mean every
// feature failing later with a different error. It returns an error, main turns
// that into exit 1, and until this file existed `daycore install` produced a
// directory that could not boot — it created config/ and never put anything in
// it. The distribution story is one binary plus `daycore install`; a required
// file the binary cannot produce breaks exactly that story.
//
// # Boundary
//
// This is a SEED, not a floor. Unlike the prompt templates and the hard
// boundaries — which are read from the embed at every startup, with the disk
// merely overlaying them — this content is written to config/models.yaml once
// and then belongs to the operator. Nothing reads it again. Making it a floor
// would mean a catalog entry the operator deleted coming back on next boot.
//
// # Tradeoff
//
// The upstream model names in here go stale (the file says so itself), so an
// install six months from now seeds a catalog naming models that may have been
// retired. The alternative — fetching a current list at install time — makes
// installation require network access to a service this project does not run.
// A stale seed the operator edits beats an install that fails offline.
//
//go:embed seed/models.yaml
var catalogSeed []byte

// CatalogSeed returns the starter model catalog written by `daycore install`.
func CatalogSeed() []byte { return catalogSeed }
