package rapport

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"daycore/internal/domain"
)

type goldenFile struct {
	Ledger []struct {
		ID       string `json:"id"`
		Action   string `json:"action"`
		Domain   string `json:"domain"`
		Actor    string `json:"actor"`
		TargetID string `json:"targetId"`
	} `json:"ledger"`
	Versions []struct {
		Version int `json:"version"`
		Scores  map[string]struct {
			Value    float64 `json:"value"`
			Evidence int     `json:"evidence"`
		} `json:"scores"`
	} `json:"versions"`
}

// The guard that makes FoldVersion mean something. Without it the constant is a
// promise kept by whoever remembers, and the failure mode of a forgotten bump is
// silent: every cached reading in the fleet keeps a number produced by rules
// that no longer exist, and nothing ever recomputes it because the version still
// matches.
//
// Both directions fail here, and the message says which:
//
//   - The fold changed and the version did not. The recorded scores no longer
//     match what the code produces.
//   - The version changed and nothing recorded what it produces. There is no
//     entry to compare against.
func TestFoldGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "fold-golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var g goldenFile
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parse golden: %v", err)
	}

	ops := make([]domain.OperationLog, 0, len(g.Ledger))
	for _, l := range g.Ledger {
		ops = append(ops, domain.OperationLog{
			ID: l.ID, Action: l.Action, Domain: l.Domain,
			Actor: l.Actor, TargetID: l.TargetID,
		})
	}

	var want map[string]struct {
		Value    float64 `json:"value"`
		Evidence int     `json:"evidence"`
	}
	found := false
	for _, v := range g.Versions {
		if v.Version == FoldVersion {
			want, found = v.Scores, true
			break
		}
	}
	if !found {
		t.Fatalf("FoldVersion is %d and testdata/fold-golden.json has no entry for it.\n"+
			"If the fold rules changed, this bump is right — record what the new rules "+
			"produce by adding a versions[] entry. If they did not change, the bump is "+
			"the mistake.", FoldVersion)
	}

	got := Replay(ops)
	for _, d := range Domains {
		w, ok := want[d]
		if !ok {
			t.Errorf("golden v%d has no score for domain %q", FoldVersion, d)
			continue
		}
		g := got.Get(d)
		if math.Abs(g.Value-w.Value) > 1e-9 || g.Evidence != w.Evidence {
			t.Errorf("domain %s: fold gives {%.4f, %d}, golden v%d records {%.4f, %d}.\n"+
				"If this change to the fold was intended, bump FoldVersion and add a new "+
				"versions[] entry rather than editing this one — an edited entry erases the "+
				"record of what the old version did, and the caches in the field were "+
				"produced by it.",
				d, g.Value, g.Evidence, FoldVersion, w.Value, w.Evidence)
		}
	}
}

// Resume decides what a cached reading is worth. The case that matters is the
// one that is easy to get half right: a reading from another version must lose
// its cursor as well as its scores.
//
// Keeping the cursor is the tempting shortcut — it looks like "we already know
// how far we got". What it actually says is "fold forward from here over scores
// of zero", so every operation before that point is skipped, the rebuilt
// reading is permanently low, and because it gets stamped with the current
// version the next read takes the same path. It never heals. sqlstore's rapport
// Get carries the same warning for the same reason.
func TestResumeDiscardsCursorWithScores(t *testing.T) {
	live := domain.OpLogCursor{CreatedAt: time.Unix(1700000000, 0).UTC(), ID: "op-9"}
	stored := &domain.RapportState{
		SessionID: "s1",
		Scores: map[string]domain.RapportScore{
			domain.OpDomainSchedule: {Value: 0.80, Evidence: 40},
		},
		Cursor:      live,
		FoldVersion: FoldVersion,
	}

	t.Run("current version resumes where it left off", func(t *testing.T) {
		f, c := Resume(stored, nil)
		if c != live {
			t.Errorf("cursor = %+v, want %+v", c, live)
		}
		if got := f.Scores().Get(domain.OpDomainSchedule).Value; got != 0.80 {
			t.Errorf("schedule = %v, want the cached 0.80", got)
		}
	})

	t.Run("another version starts over completely", func(t *testing.T) {
		old := *stored
		old.FoldVersion = FoldVersion + 1 // could be older or newer; both are unreadable
		f, c := Resume(&old, nil)
		if c != (domain.OpLogCursor{}) {
			t.Errorf("cursor = %+v, want zero — a kept cursor skips the ledger before it forever", c)
		}
		if got := f.Scores().Get(domain.OpDomainSchedule).Value; got != Cold()[domain.OpDomainSchedule].Value {
			t.Errorf("schedule = %v, want the cold-start %v", got, Cold()[domain.OpDomainSchedule].Value)
		}
	})

	t.Run("version zero is a pre-versioning row, not version zero", func(t *testing.T) {
		old := *stored
		old.FoldVersion = 0
		_, c := Resume(&old, nil)
		if c != (domain.OpLogCursor{}) {
			t.Errorf("cursor = %+v, want zero", c)
		}
	})

	t.Run("no row at all is a cold start", func(t *testing.T) {
		f, c := Resume(nil, nil)
		if c != (domain.OpLogCursor{}) || f == nil {
			t.Errorf("nil state should fold from cold: cursor %+v", c)
		}
	})

	t.Run("a matching version with no cursor still starts over", func(t *testing.T) {
		// Scores without a cursor cannot be caught up from — there is nowhere to
		// resume. Treating it as "resume from the beginning" would double-count
		// the whole ledger on top of the cached scores.
		noCursor := *stored
		noCursor.Cursor = domain.OpLogCursor{}
		f, c := Resume(&noCursor, nil)
		if c != (domain.OpLogCursor{}) {
			t.Errorf("cursor = %+v, want zero", c)
		}
		if got := f.Scores().Get(domain.OpDomainSchedule).Value; got != Cold()[domain.OpDomainSchedule].Value {
			t.Errorf("schedule = %v, want cold — resuming cached scores from the start double-counts", got)
		}
	})
}

// Snapshot and FromStored are the only place the two score shapes meet, so a
// round trip that lost evidence or a domain would be invisible everywhere else.
func TestSnapshotRoundTrip(t *testing.T) {
	in := Cold()
	in[domain.OpDomainCare] = Score{Domain: domain.OpDomainCare, Value: 0.61, Evidence: 7}
	cursor := domain.OpLogCursor{CreatedAt: time.Unix(1700000000, 0).UTC(), ID: "op-3"}

	state := Snapshot("s1", in, cursor)
	if state.FoldVersion != FoldVersion {
		t.Errorf("snapshot stamped version %d, want %d", state.FoldVersion, FoldVersion)
	}
	if state.Cursor != cursor {
		t.Errorf("cursor did not survive: %+v", state.Cursor)
	}
	out := FromStored(state.Scores)
	for _, d := range Domains {
		a, b := in.Get(d), out.Get(d)
		if a.Value != b.Value || a.Evidence != b.Evidence {
			t.Errorf("domain %s: in {%.4f, %d}, out {%.4f, %d}", d, a.Value, a.Evidence, b.Value, b.Evidence)
		}
	}
}
