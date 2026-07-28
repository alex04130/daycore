package mongostore

import (
	"testing"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
)

func TestProbeZeroTimeThroughBSON(t *testing.T) {
	in := rhythmProfileDoc{
		SessionID: "s1", Wake: "07:30", Sleep: "22:30", Source: "default",
		UpdatedAt: time.Now().UTC(),
	}
	b, err := bson.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var raw bson.M
	_ = bson.Unmarshal(b, &raw)
	t.Logf("PROBE encoded doc = %v", raw)
	var out rhythmProfileDoc
	if err := bson.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	t.Logf("PROBE runSince=%v isZero=%v lastSignal=%v isZero=%v",
		out.RunSince, out.RunSince.IsZero(), out.LastSignalAt, out.LastSignalAt.IsZero())

	// Save() does not use the struct: it $sets the raw time.Time. Emulate that.
	set := bson.M{"run_since": time.Time{}, "last_signal_at": time.Time{}}
	sb, err := bson.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	var back rhythmProfileDoc
	if err := bson.Unmarshal(sb, &back); err != nil {
		t.Fatal(err)
	}
	t.Logf("PROBE via $set path runSince=%v isZero=%v", back.RunSince, back.RunSince.IsZero())
}

func TestProbeSliceNilVsEmptyThroughBSON(t *testing.T) {
	in := docFromProposal(domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t",
		ExpiresAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	b, err := bson.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var raw bson.M
	_ = bson.Unmarshal(b, &raw)
	t.Logf("PROBE rows_json raw=%#v ops_json raw=%#v applied raw=%#v", raw["rows_json"], raw["ops_json"], raw["applied_op_ids"])
	var out proposalDoc
	if err := bson.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	p := proposalFromDoc(out)
	t.Logf("PROBE Rows nil=%v len=%d ; Ops nil=%v ; Applied nil=%v ; Accept nil=%v",
		p.Rows == nil, len(p.Rows), p.Ops == nil, p.AppliedOpIDs == nil, p.AcceptOpIDs == nil)
	t.Logf("PROBE Dur=%v ; DeliveredAt=%v", p.Dur, p.DeliveredAt)
}

func TestProbeArgsTypesThroughBSON(t *testing.T) {
	in := docFromProposal(domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t",
		Ops: []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{
			"minutes": 45,
			"big":     int64(9007199254740993),
			"nested":  map[string]any{"title": "跑步"},
			"list":    []any{1, "two"},
		}}},
		ExpiresAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	b, err := bson.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out proposalDoc
	if err := bson.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	p := proposalFromDoc(out)
	for k, v := range p.Ops[0].Args {
		t.Logf("PROBE mongo arg %q -> %v of type %T", k, v, v)
	}
	var raw bson.M
	_ = bson.Unmarshal(b, &raw)
	t.Logf("PROBE raw ops = %#v", raw["ops_json"])
}

func TestProbeRowKeysThroughBSON(t *testing.T) {
	in := docFromProposal(domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t",
		Rows:      []domain.ProposalRow{{ID: "r1", Label: "一", State: domain.ProposalPending}},
		ExpiresAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	b, _ := bson.Marshal(in)
	var raw bson.M
	_ = bson.Unmarshal(b, &raw)
	t.Logf("PROBE row doc keys = %#v", raw["rows_json"])
}

func TestProbeRapportScoresKeys(t *testing.T) {
	b, _ := bson.Marshal(bson.M{"scores": map[string]domain.RapportScore{"schedule": {Value: 0.34, Evidence: 5}}})
	var raw bson.M
	_ = bson.Unmarshal(b, &raw)
	t.Logf("PROBE rapport scores = %#v", raw["scores"])
	var d rapportDoc
	_ = bson.Unmarshal(b, &d)
	t.Logf("PROBE decoded scores = %#v", d.Scores)
}
