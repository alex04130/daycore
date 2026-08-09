//go:build !lite

package resources

import (
	"embed"
	"fmt"
	"io/fs"
)

// The embedded content. **These directives must not appear in any other file**
// — that is what makes `-tags lite` mean something, and TestLiteBuildEmbedsNothing
// enforces it.
//
//go:embed data/prompts/*/*.tmpl
//go:embed data/prompts/boundaries.json
//go:embed data/seed/models.yaml
//go:embed data/seed/oauth.yaml
//go:embed data/seed/providers.yaml
var embedded embed.FS

// Lite reports whether this binary was built with -tags lite.
const Lite = false

// FS returns the resource tree rooted so that paths are "prompts/…" and
// "seed/…" in both builds. Without the sub, every caller would have to know
// which build it is in to write a path — exactly the knowledge this package
// exists to hold.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "data")
	if err != nil {
		// Unreachable: the directives above guarantee data/ exists. A panic is
		// right anyway — a binary whose own embedded tree is unreadable cannot
		// serve a single prompt.
		panic("resources: embedded tree is unreadable: " + err.Error())
	}
	return sub
}

func describeMissing(name string, err error) error {
	return fmt.Errorf("resources: %s is missing from the embedded tree (this binary is broken; rebuild it): %w", name, err)
}
