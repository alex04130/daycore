//go:build lite

package resources

import (
	"fmt"
	"io/fs"
	"os"
)

// Lite reports whether this binary was built with -tags lite.
const Lite = true

// FS reads from DataDir(). No embed directive appears in this file, and none
// may appear anywhere outside resources_full.go — that is the entire meaning of
// this build tag.
func FS() fs.FS { return os.DirFS(DataDir()) }

// describeMissing says where the file was looked for and how it gets there.
//
// A lite binary with no data directory is the expected first-run mistake, so
// this message is the whole of the recovery path: it names the directory, the
// variable that overrides it, and the two ways to populate it. An error that
// only said "no such file" would send the reader into the source tree.
func describeMissing(name string, err error) error {
	return fmt.Errorf(
		"resources: cannot read %s from %s — this is a lite build, which embeds nothing. "+
			"Unpack the release data pack next to the binary, run `daycore install -fetch`, "+
			"or point %s at an existing deployment: %w",
		name, DataDir(), DataDirEnvKey, err)
}
