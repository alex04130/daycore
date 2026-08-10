// Package resources is the one place that decides where the files a Daycore
// binary needs come from: compiled in, or read off disk.
//
// # Two builds, one code path
//
//	default        every resource is embedded; the binary is self-sufficient
//	-tags lite     nothing is embedded; every resource is read from DataDir()
//
// Every consumer calls FS() and neither knows nor cares which build it is in.
// That is the point: the alternative is `if lite` scattered through the prompt
// loader, the boundary loader and the installer, and three places that have to
// agree about a thing only one of them can see.
//
// # What "lite" is for
//
// A deployment where the content is meant to be operated on rather than shipped
// — edited in place, diffed, put under configuration management, translated —
// and where a binary that also carries its own copy of all of it is a second
// source of truth nobody asked for. Under lite there is no floor to fall back
// to, so a missing file is an error with a path in it rather than a silent
// fallback to something the operator did not know existed.
//
// ⚠️ A previous `-tags lite` claimed exactly this and delivered none of it: the
// //go:embed directive lived in a file with no build tag, so the content
// shipped either way, and the only thing the tag changed was making the
// installer's extraction a no-op. The measured saving was 20 KiB out of 23.5 MB.
// If you are editing this package, the invariant to keep is that the embed
// directives exist ONLY in resources_full.go — TestLiteBuildEmbedsNothing
// checks it, because a tag that lies is worse than no tag.
//
// # Where lite reads from
//
//  1. DAYCORE_DATA_DIR, if set
//  2. <directory of the executable>/data
//  3. ./data
//
// Three, because there are three legitimate ways the files get there and all of
// them are the same binary: unpacked from the release tarball next to the
// executable (2), written by a full binary's `daycore install` and then pointed
// at (1), or fetched by `daycore install -fetch` (1). The order puts the
// explicit setting first and the working directory last, so a stray ./data in
// somebody's home directory never wins over a real deployment.
package resources

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// DataDirEnvKey is the environment variable that overrides the resource root.
const DataDirEnvKey = "DAYCORE_DATA_DIR"

var (
	mu      sync.RWMutex
	dataDir string
	dirOnce sync.Once
)

// SetDataDir overrides the resource root for this process. Only meaningful in a
// lite build; in a full build it is accepted and ignored, so a flag can be
// wired once instead of conditionally.
func SetDataDir(dir string) {
	mu.Lock()
	dataDir = dir
	mu.Unlock()
}

// DataDir reports the resource root a lite build reads from.
func DataDir() string {
	mu.RLock()
	d := dataDir
	mu.RUnlock()
	if d != "" {
		return d
	}
	dirOnce.Do(func() {
		found := ""
		if v := os.Getenv(DataDirEnvKey); v != "" {
			found = v
		} else if exe, err := os.Executable(); err == nil {
			if cand := filepath.Join(filepath.Dir(exe), "data"); isDir(cand) {
				found = cand
			}
		}
		if found == "" {
			found = "data"
		}
		mu.Lock()
		if dataDir == "" {
			dataDir = found
		}
		mu.Unlock()
	})
	mu.RLock()
	defer mu.RUnlock()
	return dataDir
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Read is FS().ReadFile with an error that names the build.
//
// The message matters: in a full build a missing resource is a broken binary
// and there is nothing the operator can do, while in a lite build it is a
// missing file with an address. Returning the same sentence for both would send
// half the readers looking in the wrong place.
func Read(name string) ([]byte, error) {
	b, err := fs.ReadFile(FS(), name)
	if err != nil {
		return nil, describeMissing(name, err)
	}
	return b, nil
}
