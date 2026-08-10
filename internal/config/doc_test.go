package config

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// docs/CONFIG.md's tables are generated from Settings, and this keeps them
// current — the same shape as docs/API_SURFACE.md, and for the same reason.
//
// A hand-maintained configuration table is a hand-maintained list, and this repo
// has now been bitten by three of those: the twelve-entry revert list that was
// really sixteen, the twelve-action reversibility test that checked a subset of
// itself, and the dynamic-action sites. A config table going stale is worse than
// those, because the console reads it as the truth about what it may edit.
var updateDoc = flag.Bool("update", false, "rewrite the generated tables in docs/CONFIG.md")

const (
	configDoc   = "../../docs/CONFIG.md"
	configBegin = "<!-- BEGIN GENERATED CONFIG TABLE -->\n"
	configEnd   = "<!-- END GENERATED CONFIG TABLE -->"
)

func renderConfigTables() string {
	var b strings.Builder
	for _, layer := range []Layer{LayerBoot, LayerRuntime} {
		switch layer {
		case LayerBoot:
			b.WriteString("\n### 启动期（只走环境变量，控制台只读）\n\n")
		case LayerRuntime:
			b.WriteString("\n### 运行时（可热改；环境变量是种子）\n\n")
		}
		b.WriteString("| 环境变量 | Config 字段 | 密钥 | 为什么 / 注意 |\n|---|---|---|---|\n")
		for _, s := range Settings {
			if s.Layer != layer || s.Env == "" {
				continue
			}
			secret := ""
			if s.Secret {
				secret = "🔑"
			}
			why := strings.ReplaceAll(s.Why, "|", "\\|")
			if why == "" {
				why = "—"
			}
			b.WriteString("| `" + s.Env + "` | `" + s.Field + "` | " + secret + " | " + why + " |\n")
		}
	}
	return b.String()
}

func TestConfigDocIsCurrent(t *testing.T) {
	raw, err := os.ReadFile(configDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	i, j := strings.Index(doc, configBegin), strings.Index(doc, configEnd)
	if i < 0 || j < 0 || j < i {
		t.Fatal("the generated-table markers are missing from docs/CONFIG.md — hand-edited back? Run `make config-doc`.")
	}
	fresh := renderConfigTables()
	if *updateDoc {
		if err := os.WriteFile(configDoc, []byte(doc[:i]+configBegin+fresh+doc[j:]), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("regenerated %s", configDoc)
		return
	}
	if got := doc[i+len(configBegin) : j]; got != fresh {
		t.Errorf("docs/CONFIG.md is out of date — run `make config-doc`.\n\nwant:\n%s\n\ngot:\n%s", fresh, got)
	}
}
