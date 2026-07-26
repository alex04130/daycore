package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// lockRules mirrors the parts of api/lock-rules.json the Go side consumes. The
// fixture is the shared contract with the frontends — the test reads it rather
// than restating the cases, so a rule that changes in one place cannot quietly
// stay stale in the other.
type lockRules struct {
	ClassTitle struct {
		Pattern   string `json:"pattern"`
		TrimInput bool   `json:"trimInput"`
	} `json:"classTitle"`
	DefaultReason map[string]map[string]string `json:"defaultReason"`
	Cases         []struct {
		Title string `json:"title"`
		Type  string `json:"type"`
		Level string `json:"level"`
		Why   string `json:"why"`
	} `json:"cases"`
	KnownDivergences []struct {
		Case struct {
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"case"`
		JS string `json:"js"`
		Go string `json:"go"`
	} `json:"knownDivergences"`
}

func loadLockRules(t *testing.T) lockRules {
	t.Helper()
	path := filepath.Join("..", "..", "api", "lock-rules.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var r lockRules
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return r
}

// The pattern in the fixture must be the one the code compiles. If they drift,
// the frontends are following a rule the backend does not enforce.
func TestLockRulesPatternMatchesCode(t *testing.T) {
	r := loadLockRules(t)
	if r.ClassTitle.Pattern != ClassTitlePattern {
		t.Errorf("fixture pattern and ClassTitlePattern differ:\n fixture: %s\n code:    %s",
			r.ClassTitle.Pattern, ClassTitlePattern)
	}
	if !r.ClassTitle.TrimInput {
		t.Error("fixture says trimInput=false but IsClassTitle trims")
	}
}

func TestLockRulesDefaultReasons(t *testing.T) {
	r := loadLockRules(t)
	for level, want := range map[LockLevel]map[string]string{
		LockHard: r.DefaultReason["hard"],
		LockSoft: r.DefaultReason["soft"],
	} {
		for locale, text := range want {
			if got := DefaultLockReason(level, locale); got != text {
				t.Errorf("DefaultLockReason(%q, %q) = %q, fixture says %q", level, locale, got, text)
			}
		}
	}
	if got := DefaultLockReason(LockNone, "zh-CN"); got != "" {
		t.Errorf("an unlocked block should carry no derived reason, got %q", got)
	}
	// Unknown locales fall back to the design original rather than to English.
	if got := DefaultLockReason(LockHard, "fr"); got != r.DefaultReason["hard"]["zh-CN"] {
		t.Errorf("unknown locale should fall back to zh-CN, got %q", got)
	}
}

// The shared case table — the same rows the TypeScript side must pass.
func TestLockRulesCases(t *testing.T) {
	r := loadLockRules(t)
	if len(r.Cases) < 15 {
		t.Fatalf("fixture has only %d cases; the table is meant to be the real coverage", len(r.Cases))
	}
	for _, c := range r.Cases {
		got := InferLock(BlockType(c.Type), c.Title)
		if string(got) != c.Level {
			t.Errorf("InferLock(%q, %q) = %q, want %q — %s", c.Type, c.Title, got, c.Level, c.Why)
		}
	}
}

// Divergences are recorded so nobody "fixes" the pattern later and forks it
// from the design original. This asserts Go still behaves the documented way.
func TestLockRulesKnownDivergences(t *testing.T) {
	r := loadLockRules(t)
	for _, d := range r.KnownDivergences {
		got := InferLock(BlockType(d.Case.Type), d.Case.Title)
		if string(got) != d.Go {
			t.Errorf("divergence case %q: Go gives %q, fixture documents %q (JS: %q)",
				d.Case.Title, got, d.Go, d.JS)
		}
	}
}

// DeriveLock is idempotent and must never overwrite a deliberate choice —
// otherwise every read would re-lock a class the user unlocked by hand, and
// toggle-lock would do nothing that survives a refresh.
func TestDeriveLockRespectsDeliberateChoices(t *testing.T) {
	unlocked := &TimeBlock{
		Title: "CS 201 数据结构（课）", Type: BlockAppointment,
		LockLevel: LockNone, LockSource: LockSourceUser,
	}
	if DeriveLock(unlocked, "zh-CN") {
		t.Error("DeriveLock changed a user-set lock")
	}
	if unlocked.LockLevel != LockNone {
		t.Errorf("user unlock was overwritten: %q", unlocked.LockLevel)
	}

	fresh := &TimeBlock{Title: "CS 201 数据结构（课）", Type: BlockAppointment}
	if !DeriveLock(fresh, "zh-CN") {
		t.Fatal("DeriveLock reported no change on an underived block")
	}
	if fresh.LockLevel != LockHard || fresh.LockSource != LockSourceDerived {
		t.Errorf("derived to level=%q source=%q", fresh.LockLevel, fresh.LockSource)
	}
	if DeriveLock(fresh, "zh-CN") {
		t.Error("DeriveLock is not idempotent — second call reported a change")
	}
}

// RederiveLock is the opt-in form: a derived lock follows edits to the block,
// a deliberate one does not.
func TestRederiveLock(t *testing.T) {
	// appointment → task should drop the lock
	b := &TimeBlock{Title: "高等数学课", Type: BlockAppointment}
	DeriveLock(b, "zh-CN")
	b.Type = BlockTask
	if !RederiveLock(b, "zh-CN") {
		t.Error("RederiveLock reported no change after the type changed")
	}
	if b.LockLevel != LockNone || b.LockReason != "" {
		t.Errorf("stale lock survived a type change: level=%q reason=%q", b.LockLevel, b.LockReason)
	}

	// a user-set lock is a decision, not a guess
	kept := &TimeBlock{
		Title: "写作业", Type: BlockTask,
		LockLevel: LockHard, LockReason: "别动", LockSource: LockSourceUser,
	}
	if RederiveLock(kept, "zh-CN") {
		t.Error("RederiveLock overwrote a user-set lock")
	}
	if kept.LockLevel != LockHard || kept.LockReason != "别动" {
		t.Errorf("user lock mangled: level=%q reason=%q", kept.LockLevel, kept.LockReason)
	}

	// no-op when nothing actually changed
	stable := &TimeBlock{Title: "社团例会", Type: BlockAppointment}
	DeriveLock(stable, "zh-CN")
	if RederiveLock(stable, "zh-CN") {
		t.Error("RederiveLock reported a change when nothing moved")
	}
}

func TestMovable(t *testing.T) {
	cases := []struct {
		level     LockLevel
		actor     string
		confirmed bool
		want      bool
		why       string
	}{
		{LockHard, ActorUser, false, false, "hard blocks the user"},
		{LockHard, ActorUser, true, false, "hard is not confirmable away"},
		{LockSoft, ActorUser, false, false, "soft needs confirmation"},
		{LockSoft, ActorUser, true, true, "soft yields to confirmation"},
		{LockNone, ActorUser, false, true, "free block"},
		{LockUnset, ActorUser, false, true, "never-derived behaves as free"},
		{LockHard, ActorAgent, false, true, "the agent plans around locks, not blocked by them"},
		{LockSoft, ActorAgent, false, true, "same for soft"},
		{LockHard, ActorSystem, false, true, "system must pass or revert could not undo a lock change"},
	}
	for _, c := range cases {
		b := TimeBlock{LockLevel: c.level}
		if got := b.Movable(c.actor, c.confirmed); got != c.want {
			t.Errorf("Movable(level=%q actor=%q confirmed=%v) = %v, want %v — %s",
				c.level, c.actor, c.confirmed, got, c.want, c.why)
		}
	}
}
