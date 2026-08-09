package setup

import (
	"io/fs"

	"daycore/internal/resources"
)

// Where the installer's own material comes from. One indirection so that the
// sections never mention a build tag: under `-tags lite` these read the data
// directory, under a normal build the embedded tree, and runModels looks the
// same either way.

// DataDirEnvKey is re-exported so the installer can name it in a message
// without importing the resources package into every file that prints one.
const DataDirEnvKey = resources.DataDirEnvKey

// CatalogSeedBytes is the starter model catalog `daycore install` writes.
//
// A SEED, not a floor: unlike the prompt templates and the boundary block —
// read from resources at every startup with the disk merely overlaying them —
// this is written once and then belongs to the operator. Nothing reads it
// again. Making it a floor would mean a model the operator deleted coming back
// on the next boot.
func CatalogSeedBytes() ([]byte, error) { return resources.Read("seed/models.yaml") }

// OAuthSeedBytes is the placeholder OAuth file.
//
// Written even when the operator configures nothing, because a capability
// nobody can find is the same as one that does not exist: holding one binary,
// there is no way to learn that Google and GitHub are presets needing two
// fields each. It is inert — LoadOAuthProviders skips any provider with an
// empty client_id — which is the property that makes writing it safe. A
// placeholder that half-registered a provider would hand users a login button
// that fails.
func OAuthSeedBytes() ([]byte, error) { return resources.Read("seed/oauth.yaml") }

// Preflight fails before the first question when the resources this run needs
// are not reachable.
//
// Fail EARLY, not at the end. The first version of the lite path asked all
// eight sections, wrote config/models.yaml from a nil seed — an empty file that
// LoadCatalog rejects and that a re-run then SKIPS because it exists — and only
// then reported that it had no resources. That is a permanently broken
// deployment produced by a command that had everything it needed to refuse in
// its first second.
func Preflight() error {
	for _, name := range []string{"seed/models.yaml", "seed/oauth.yaml", "prompts/boundaries.json"} {
		if _, err := resources.Read(name); err != nil {
			return err
		}
	}
	return nil
}

// PromptResources is the tree ExtractPrompts walks, or nil when a lite build
// has no data directory.
func PromptResources() fs.FS {
	f := resources.FS()
	if _, err := fs.Stat(f, "prompts"); err != nil {
		return nil
	}
	return f
}

// DataDirForEnv returns the resource root to record in the generated .env, or
// "" in a full build where there is nothing to record.
//
// Written for lite builds specifically: that binary cannot start without
// finding its data, and the .env is the one file an operator is guaranteed to
// look at. Leaving it implicit means a working deployment breaks the first time
// somebody starts it from a different working directory.
func DataDirForEnv() string {
	if !resources.Lite {
		return ""
	}
	return resources.DataDir()
}
