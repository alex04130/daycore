package mongostore

import (
	"testing"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
)

// This package had no tests at all, which is how a whole class of divergence
// from sqlstore went unnoticed: the two stores agree on the schema and disagree
// on what comes back out of it.
//
// BSON marshalling needs no server, so the parts of that class that are pure
// serialisation are testable here — and those are the ones that bite, because
// the service layer above will be written against whichever store the author
// happened to run.

// The one that mattered most. ProposalOp.Args is map[string]any, so its values
// are re-typed by whichever serialiser the backend uses: a 45 comes back as
// float64 through encoding/json and as int32 through BSON, a list as
// []interface{} versus primitive.A. An agent tool doing
// args["minutes"].(float64) is correct on SQL and a failed assertion on Mongo.
//
// Both stores therefore put ops and rows through encoding/json — which is why
// the bson tags say json — and both are equally lossy in the same places.
func TestProposalOpArgsRoundTripWithJSONTypes(t *testing.T) {
	in := domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t", Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
		Ops: []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{
			"minutes": 45,
			"nested":  map[string]any{"title": "跑步"},
			"list":    []any{1, "two"},
		}}},
		Rows: []domain.ProposalRow{{ID: "r1", Label: "一", State: domain.ProposalPending}},
	}
	doc, err := docFromProposal(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var back proposalDoc
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	out := proposalFromDoc(back)

	if len(out.Ops) != 1 {
		t.Fatalf("ops: %+v", out.Ops)
	}
	args := out.Ops[0].Args
	if _, ok := args["minutes"].(float64); !ok {
		t.Errorf("minutes came back as %T, want float64 — the same type the SQL store yields", args["minutes"])
	}
	if _, ok := args["list"].([]any); !ok {
		t.Errorf("list came back as %T, want []any (BSON would give primitive.A)", args["list"])
	}
	nested, ok := args["nested"].(map[string]any)
	if !ok || nested["title"] != "跑步" {
		t.Errorf("nested came back as %T = %v", args["nested"], args["nested"])
	}
	if len(out.Rows) != 1 || out.Rows[0].Label != "一" {
		t.Errorf("rows: %+v", out.Rows)
	}
}

// A nil slice must come back as an empty one, not nil, on both stores — the
// write path normalises it so no caller needs a nil check the round trip already
// promised to remove.
func TestNilSlicesComeBackEmpty(t *testing.T) {
	doc, err := docFromProposal(domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t", Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := bson.Marshal(doc)
	var back proposalDoc
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	out := proposalFromDoc(back)
	if out.Rows == nil || len(out.Rows) != 0 {
		t.Errorf("Rows = %v, want an empty slice", out.Rows)
	}
	if out.Ops == nil || len(out.Ops) != 0 {
		t.Errorf("Ops = %v, want an empty slice", out.Ops)
	}
	if out.AppliedOpIDs == nil || out.AcceptOpIDs == nil {
		t.Errorf("op-id slices came back nil: %v / %v", out.AppliedOpIDs, out.AcceptOpIDs)
	}
	// The three nullable timestamps stay nil, which is what "still in the pool,
	// never shown" means.
	if out.DeliveredAt != nil || out.PushedAt != nil || out.DeliverAfter != nil {
		t.Errorf("nil timestamps came back set: %+v", out)
	}
}

// A rhythm profile with no current awake stretch stores zero times, and they
// must read back as zero — the Protector reads a zero RunSince as "asleep", so a
// spurious epoch value would read as "awake since 1970".
func TestZeroTimesStayZeroThroughBSON(t *testing.T) {
	raw, err := bson.Marshal(rhythmProfileDoc{
		SessionID: "s1", Wake: "07:30", Sleep: "22:30", Source: "default",
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var back rhythmProfileDoc
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !back.RunSince.IsZero() || !back.LastSignalAt.IsZero() {
		t.Errorf("run marks came back non-zero: %v / %v", back.RunSince, back.LastSignalAt)
	}
}

// Rapport scores are a map keyed by domain name, and the domain names have to
// survive as-is — they are the join between this cache and the operation log.
func TestRapportScoreKeysSurvive(t *testing.T) {
	raw, err := bson.Marshal(rapportDoc{
		SessionID: "s1",
		Scores: map[string]domain.RapportScore{
			domain.OpDomainSchedule: {Value: 0.34, Evidence: 5},
			domain.OpDomainArchive:  {Value: 0.61, Evidence: 12},
		},
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var back rapportDoc
	if err := bson.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.Scores[domain.OpDomainSchedule]; got.Value != 0.34 || got.Evidence != 5 {
		t.Errorf("schedule = %+v", got)
	}
	if got := back.Scores[domain.OpDomainArchive]; got.Evidence != 12 {
		t.Errorf("archive = %+v", got)
	}
}

// A job run's _id is the OCCURRENCE and claim_id is the rotating claim. Mixing
// them up is what let a zombie instance's late Finish mark a healthy run failed.
func TestJobRunKeyIsTheOccurrence(t *testing.T) {
	k1 := jobRunKey("s1", domain.JobMorningBrief, "2026-07-27")
	k2 := jobRunKey("s1", domain.JobMorningBrief, "2026-07-28")
	k3 := jobRunKey("s2", domain.JobMorningBrief, "2026-07-27")
	if k1 == k2 || k1 == k3 {
		t.Errorf("distinct occurrences collided: %q %q %q", k1, k2, k3)
	}
	if k1 != jobRunKey("s1", domain.JobMorningBrief, "2026-07-27") {
		t.Error("the key is not stable for the same occurrence")
	}
}

// A proposal whose ops will not marshal must fail the write rather than be
// stored claiming work it does not hold — the same rule as the SQL side, where
// marshalJSON's "null" fallback would have written a live-looking card with no
// ops at all.
func TestUnmarshalableOpsFailTheWrite(t *testing.T) {
	_, err := docFromProposal(domain.Proposal{
		ID: "p1", SessionID: "s1", Title: "t", Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
		Ops: []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{
			"bad": func() {}, // json.Marshal refuses a func
		}}},
	})
	if err == nil {
		t.Error("want an error rather than a card that claims work it does not hold")
	}
}

// The connection string is parsed by the driver, not by slicing the string. The
// hand-rolled version cut at the first '/' after "://" and returned a database
// named "ss@host/mydb" for `mongodb://user:p/ss@host/mydb` — the same class of
// mistake the MySQL DSN handling made, and the same silent outcome: the server
// creates and uses a nonsense database.
func TestDatabaseNameFromConnectionString(t *testing.T) {
	for _, c := range []struct{ dsn, want string }{
		{"mongodb://127.0.0.1:27017/daycore", "daycore"},
		{"mongodb://127.0.0.1:27017/mydb?retryWrites=true&w=majority", "mydb"},
		{"mongodb://user:pass@127.0.0.1:27017/mydb?authSource=admin", "mydb"},
		{"mongodb://a:27017,b:27017/mydb?replicaSet=rs0", "mydb"},
		// mongodb+srv must work without resolving anything: the driver's own
		// connstring parser does an SRV lookup, which would put a DNS round trip
		// in every Open and fail where the record cannot be resolved.
		{"mongodb+srv://user:pass@cluster.example.com/mydb?retryWrites=true", "mydb"},
		// A percent-encoded '/' in the password is the spec-conformant form and
		// must not be mistaken for the path separator.
		{"mongodb://user:p%2Fss@127.0.0.1:27017/mydb", "mydb"},
		// No database named: fall back rather than guess.
		{"mongodb://127.0.0.1:27017", DefaultDatabase},
		{"mongodb://127.0.0.1:27017/", DefaultDatabase},
		{"mongodb://127.0.0.1:27017/?directConnection=true", DefaultDatabase},
		// Unparseable: fall back rather than connect somewhere arbitrary.
		{"not a connection string", DefaultDatabase},
		{"mongodb://user:p/ss@127.0.0.1:27017/mydb", DefaultDatabase},
	} {
		if got := dbNameFromDSN(c.dsn); got != c.want {
			t.Errorf("dbNameFromDSN(%q) = %q, want %q", c.dsn, got, c.want)
		}
	}
}
