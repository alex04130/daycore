package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"daycore/internal/config"
)

// Runtime-classified settings must be read through s.runtime(), never s.cfg.
//
// # The failure
//
// s.cfg is the BOOT config: the values as the environment supplied them, before
// any console override. Reading a runtime knob from it produces the single worst
// outcome the configuration layering exists to prevent — the operator changes a
// limit, the console says it saved, and the process keeps using the old number.
// Nothing errors, nothing logs, and the only evidence is behaviour.
//
// It is also the easy mistake: `s.cfg.MaxUploadBytes` reads correctly, compiles,
// and is what every line around it looks like. So it is a build failure rather
// than a convention.
func TestRuntimeFieldsAreReadThroughTheSnapshot(t *testing.T) {
	runtimeFields := map[string]bool{}
	for _, s := range config.Settings {
		if s.Layer == config.LayerRuntime {
			runtimeFields[s.Field] = true
		}
	}
	if len(runtimeFields) == 0 {
		t.Fatal("no runtime settings found — this gate would pass on an empty set")
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				// Looking for <anything>.cfg.<RuntimeField>
				outer, ok := n.(*ast.SelectorExpr)
				if !ok || !runtimeFields[outer.Sel.Name] {
					return true
				}
				inner, ok := outer.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "cfg" {
					return true
				}
				pos := fset.Position(outer.Pos())
				offenders = append(offenders,
					filepath.Join("internal/server", filepath.Base(path))+":"+itoa(pos.Line)+"  cfg."+outer.Sel.Name)
				return true
			})
		}
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("%d read(s) of a runtime setting from the BOOT config:\n\n%s\n\n"+
			"Use s.runtime() (or w.s.runtime()) instead. s.cfg holds the values as the\n"+
			"environment supplied them, so reading a runtime knob from it means a console\n"+
			"change that saves successfully and does nothing — no error, no log, only\n"+
			"behaviour. If the field genuinely cannot be hot, reclassify it in\n"+
			"internal/config/layer.go and say why.",
			len(offenders), strings.Join(offenders, "\n"))
	}
}
