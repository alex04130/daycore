package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"daycore/internal/i18n"
)

// The gate that makes the message catalog stick.
//
// Before this batch, 279 error messages were Chinese string literals inline at
// their call sites — readable, and completely untranslatable. The repo's own
// claim is that adding a language means dropping a JSON file into LOCALES_DIR;
// none of these could be reached that way, so the claim was true of the
// mechanism and false of the surface it was supposed to cover.
//
// Migrating them once fixes today. This test is what stops it happening again,
// which matters more: "remember to use the catalog" is a rule, and this repo
// has been bitten repeatedly by rules that depended on remembering.
func TestNoHardcodedUserFacingText(t *testing.T) {
	// An AST walk, not a regex over lines.
	//
	// The first version of this gate matched `s.writeErr(w, <status>, "<code>",
	// "<message>"` on a single line, and it let four messages through — three
	// wrapped in fmt.Sprintf and one whose message sat on the following line.
	// A gate with a blind spot is worse than no gate: it reports clean and the
	// rule stops being enforced by anything else.
	//
	// So: find every call to writeErr, walk its whole argument subtree, and flag
	// any string literal in it that contains a Han character. That covers
	// fmt.Sprintf wrapping, concatenation, multi-line calls, and whatever the
	// next shape turns out to be.
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
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "writeErr" {
					return true
				}
				for _, arg := range call.Args {
					ast.Inspect(arg, func(m ast.Node) bool {
						lit, ok := m.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							return true
						}
						text, err := strconv.Unquote(lit.Value)
						if err != nil || !hasCJK(text) {
							return true
						}
						pos := fset.Position(lit.Pos())
						offenders = append(offenders,
							filepath.Join("internal/server", filepath.Base(path))+":"+itoa(pos.Line)+"  "+text)
						return true
					})
				}
				return true
			})
		}
	}
	if len(offenders) > 0 {
		t.Errorf("%d user-visible message(s) bypass the catalog.\n"+
			"Register the text in messages.go and call s.writeErrL (or writeErrf for one\n"+
			"that carries a value) — a literal here cannot be translated by dropping a\n"+
			"file into LOCALES_DIR, which is the whole point of the three-layer catalog.\n\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}

// What this gate does NOT see, stated so nobody assumes it sees everything:
//
//   - Text that reaches a response by some route other than writeErr (a
//     writeJSON body, an SSE frame, a prompt). Widening it to "any Han literal
//     in the package" is not possible today: messages.go itself is nothing but
//     Han literals, and so are the tests' own fixtures.
//   - A message assembled from non-literal pieces (a variable holding Chinese).
//   - Anything outside internal/server.
//
// Those are known gaps, not oversights. If one of them starts costing
// something, the fix is another targeted walk, not a looser regex.

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// How much of a language is actually translated, as a number.
//
// It does not assert a threshold — en-US sits far below 100% today and that is
// a known, deliberate state: the 241 error messages were registered with zh-CN
// only, because authoring their English is copy work and copy is frozen by
// project rule, while the mechanism is not. What this prints is the worklist
// size, so "how far along is en-US" stops being a feeling.
//
// i18n.Export(locale) hands a translator every key with its current fallback,
// which is the file they fill in and drop into LOCALES_DIR.
func TestReportCatalogCoverage(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		have, total := i18n.Coverage(locale)
		t.Logf("%s: %d/%d keys (%d untranslated)", locale, have, total, total-have)
	}
	if have, total := i18n.Coverage("zh-CN"); have != total {
		t.Errorf("zh-CN is the source language and must be complete: %d/%d", have, total)
	}
}
