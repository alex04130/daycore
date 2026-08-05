package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"daycore/internal/domain"
)

// opOf fetches one ledger entry by id.
func opOf(t *testing.T, s *Server, sid, opID string) domain.OperationLog {
	t.Helper()
	op, err := s.store.OpLogs().Get(context.Background(), sid, opID)
	if err != nil {
		t.Fatalf("op %s not logged: %v", opID, err)
	}
	return *op
}

func TestToolAssignmentUpsertCreateAndRevert(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	res := s.toolAssignmentUpsert(ctx, sid, `{"title":"离散数学作业","dueAt":"2026-08-10"}`)
	if !res.OK {
		t.Fatalf("create: %s", res.ErrMsg)
	}
	op := opOf(t, s, sid, res.OpID)
	if op.Action != "assignment_upsert" || op.Actor != domain.ActorAgent {
		t.Fatalf("op = %s/%s", op.Action, op.Actor)
	}

	revertOp(t, s, sid, res.OpID)
	if _, err := s.store.Assignments().Get(ctx, sid, op.TargetID); err == nil {
		t.Fatal("revert of a created assignment must delete the row")
	}
}

func TestToolAssignmentUpsertUpdateKeepsStatusAndReverts(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	created, err := s.createManualAssignment(ctx, sid, "物理实验报告", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.Assignments().SetStatus(ctx, sid, created.ID, domain.AssignmentDone); err != nil {
		t.Fatal(err)
	}

	res := s.toolAssignmentUpsert(ctx, sid, `{"id":"`+created.ID+`","dueAt":"2026-08-12"}`)
	if !res.OK {
		t.Fatalf("update: %s", res.ErrMsg)
	}
	got, err := s.store.Assignments().Get(ctx, sid, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DueAt == nil || got.DueAt.Format("2006-01-02") != "2026-08-12" {
		t.Fatalf("dueAt = %v", got.DueAt)
	}
	if got.Status != domain.AssignmentDone {
		t.Fatalf("a due-date edit must not reset the planner status, got %q", got.Status)
	}

	revertOp(t, s, sid, res.OpID)
	got, _ = s.store.Assignments().Get(ctx, sid, created.ID)
	if got.DueAt != nil {
		t.Fatalf("revert should restore dueAt=nil, got %v", got.DueAt)
	}
}

func TestToolWishAddDedupes(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	if res := s.toolWishAdd(ctx, sid, `{"text":"想学 Rust","effortMin":120}`); !res.OK {
		t.Fatalf("first add: %s", res.ErrMsg)
	}
	res := s.toolWishAdd(ctx, sid, `{"text":"  想学 rust  "}`)
	if !res.OK {
		t.Fatalf("second add: %s", res.ErrMsg)
	}
	if dup, _ := res.Data["duplicate"].(bool); !dup {
		t.Fatal("case/space-insensitive duplicate was not detected")
	}
	wishes, _ := s.store.Wishes().List(ctx, sid, "active")
	if len(wishes) != 1 {
		t.Fatalf("wish count = %d, want 1", len(wishes))
	}
	if wishes[0].EffortMin != 120 {
		t.Fatalf("effortMin = %d", wishes[0].EffortMin)
	}
}

func TestToolMoodRecordRespectsManualCheckin(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	res := s.toolMoodRecord(ctx, sid, "Asia/Shanghai", `{"mood":"tired","note":"说累死了"}`)
	if !res.OK {
		t.Fatalf("agent record: %s", res.ErrMsg)
	}
	op := opOf(t, s, sid, res.OpID)
	if op.Action != "mood_record" || op.Actor != domain.ActorAgent {
		t.Fatalf("op = %s/%s", op.Action, op.Actor)
	}
	checkins, _ := s.store.Moods().List(ctx, sid, 10)
	if len(checkins) != 1 || checkins[0].Source != domain.MoodSourceAgent {
		t.Fatalf("checkins = %+v", checkins)
	}

	// A manual check-in later the same day stands; the agent's next inference
	// must not create a second row.
	if _, err := s.store.Moods().Create(ctx, &domain.MoodCheckin{
		SessionID: sid, Mood: "calm", Source: domain.MoodSourceUser, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	res = s.toolMoodRecord(ctx, sid, "Asia/Shanghai", `{"mood":"anxious"}`)
	if !res.OK {
		t.Fatalf("second record: %s", res.ErrMsg)
	}
	if dup, _ := res.Data["alreadyCheckedIn"].(bool); !dup {
		t.Fatal("manual check-in on the day must suppress the agent's record")
	}
	checkins, _ = s.store.Moods().List(ctx, sid, 10)
	if len(checkins) != 2 {
		t.Fatalf("checkin count = %d, want 2", len(checkins))
	}

	// And the agent's own record is revertible.
	revertOp(t, s, sid, op.ID)
	if checkins, _ := s.store.Moods().List(ctx, sid, 10); len(checkins) != 1 {
		t.Fatalf("after revert, checkin count = %d, want 1 (only the manual one)", len(checkins))
	}
}

func TestToolMoodRecordRejectsUnknownMood(t *testing.T) {
	s, sid := newAgentTestServer(t)
	if res := s.toolMoodRecord(context.Background(), sid, "UTC", `{"mood":"kinda-sad"}`); res.OK {
		t.Fatal("an id outside the registry must fail so the model retries")
	}
}

func TestToolMaterialAdd(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()

	if res := s.toolMaterialAdd(ctx, sid, `{"title":"重点","body":"x","category":"nonsense"}`); res.OK {
		t.Fatal("unknown category must fail")
	}
	long := strings.Repeat("考试重点在第三章。", 100) // bodies are exempt from clamping
	res := s.toolMaterialAdd(ctx, sid, `{"title":"离散考试范围","body":"`+long+`"}`)
	if !res.OK {
		t.Fatalf("add: %s", res.ErrMsg)
	}
	op := opOf(t, s, sid, res.OpID)
	if op.Action != "material_create" || op.Actor != domain.ActorAgent {
		t.Fatalf("op = %s/%s", op.Action, op.Actor)
	}
	m, err := s.store.Materials().Get(ctx, sid, op.TargetID)
	if err != nil || m.Body != long {
		t.Fatalf("material = %v, body len %d", err, len(m.Body))
	}
	revertOp(t, s, sid, res.OpID)
	if _, err := s.store.Materials().Get(ctx, sid, op.TargetID); err == nil {
		t.Fatal("revert of material_create must delete the row")
	}
}
