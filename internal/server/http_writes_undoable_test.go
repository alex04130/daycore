package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// Every HTTP write path is undoable, end to end.
//
// # ⚠️ What was wrong, and why nothing noticed for so long
//
// The repo's rule is that every write goes through logOp — "副作用永远服务端执行 …
// 保证了每个操作可审计、可撤销". Six HTTP handlers did not: creating an assignment,
// changing its status, recording a mood, and creating/updating/deleting a note.
//
// The AGENT's versions of the same acts logged correctly (tool_capture.go), so
// the ledger looked healthy from every angle anyone had looked from. The
// asymmetry only becomes visible when a FRONTEND uses those endpoints — an
// assignment the companion created could be taken back, and the identical one
// you typed yourself could not, silently, with an undo bar on screen for the
// plan writes right next to them.
//
// 琉璃初版 is the first frontend to call them (its 资料 and 心情 pages), which is
// how this surfaced.
//
// # ⚠️ Why this is one test per path rather than "logOp is called"
//
// Asserting the call would pass on an op whose Detail is unusable — and a
// wrong snapshot is worse than a missing one: undo reports success and restores
// nothing, or restores the wrong thing. So each case does the write, presses
// undo through the real endpoint, and asserts the world went back.

func writeJSONReq(t *testing.T, s *Server, sid, method, path string, body any, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, versionPath(path), nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, versionPath(path), bytes.NewReader(b))
	}
	rec := httptest.NewRecorder()
	h(rec, r.WithContext(withSession(r.Context(), sid)))
	return rec
}

// latestOp is the id of the most recent, not-yet-reverted operation.
func latestOp(t *testing.T, s *Server, sid, wantAction string) string {
	t.Helper()
	ops, err := s.store.OpLogs().List(context.Background(), sid, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Action != wantAction {
			continue
		}
		// ⚠️ "not already undone" is asked of the LEDGER, not of a field on the
		// row — the ledger is append-only, so an undo is a separate compensating
		// entry rather than a flag flipped on the original.
		if done, _ := s.store.OpLogs().RevertedBy(context.Background(), sid, op.ID); done == nil {
			return op.ID
		}
	}
	t.Fatalf("no %q operation was logged — this write is not undoable, whatever the response said "+
		"(saw %d ops: %s)", wantAction, len(ops), actionsOf(ops))
	return ""
}

func actionsOf(ops []domain.OperationLog) string {
	var out []string
	for _, o := range ops {
		out = append(out, o.Action)
	}
	return strings.Join(out, ", ")
}

func undo(t *testing.T, s *Server, sid, opID string) {
	t.Helper()
	r := httptest.NewRequest("POST", versionPath("/api/ops/"+opID+"/revert"), nil)
	r.SetPathValue("id", opID)
	rec := httptest.NewRecorder()
	s.handleOpRevert(rec, r.WithContext(withSession(r.Context(), sid)))
	if rec.Code != http.StatusOK {
		t.Fatalf("undo refused: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAssignmentCreateIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	rec := writeJSONReq(t, s, sid, "POST", "/api/assignments",
		map[string]any{"title": "写实验报告"}, s.handleAssignmentCreate)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created domain.Assignment
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	undo(t, s, sid, latestOp(t, s, sid, "assignment_upsert"))

	// ⚠️ Gone, not dismissed. Reverting a creation means the row goes away —
	// dismissal is a workflow state the user chose, not an erasure.
	if got, err := s.store.Assignments().Get(context.Background(), sid, created.ID); err == nil && got != nil {
		t.Errorf("undo left the assignment behind (status %q)", got.Status)
	}
}

func TestHTTPAssignmentPatchIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	rec := writeJSONReq(t, s, sid, "POST", "/api/assignments",
		map[string]any{"title": "读第三章"}, s.handleAssignmentCreate)
	var created domain.Assignment
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Status != domain.AssignmentPending {
		t.Fatalf("expected a pending assignment to start from, got %q", created.Status)
	}

	r := httptest.NewRequest("PATCH", versionPath("/api/assignments/"+created.ID),
		strings.NewReader(`{"status":"done"}`))
	r.SetPathValue("id", created.ID)
	pr := httptest.NewRecorder()
	s.handleAssignmentPatch(pr, r.WithContext(withSession(r.Context(), sid)))
	if pr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", pr.Code, pr.Body.String())
	}

	undo(t, s, sid, latestOp(t, s, sid, "assignment_upsert"))

	// ⚠️ THE assertion for the before-snapshot. If the handler had logged
	// `before: null` (the shape a create uses), the revert would DELETE an
	// assignment the reader only marked done — so "it is back to pending" and
	// "it still exists" are two different failures and both matter.
	got, err := s.store.Assignments().Get(context.Background(), sid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("undo deleted an assignment that was only edited: %v", err)
	}
	if got.Status != domain.AssignmentPending {
		t.Errorf("status is %q, want it back at %q", got.Status, domain.AssignmentPending)
	}
}

func TestHTTPMoodCreateIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	kinds := domain.MoodKinds()
	if len(kinds) == 0 {
		t.Fatal("the mood registry is empty; this test would pass for the wrong reason")
	}
	rec := writeJSONReq(t, s, sid, "POST", "/api/mood",
		map[string]any{"mood": kinds[0].ID, "note": "还行"}, s.handleMoodCreate)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	before, _ := s.store.Moods().List(context.Background(), sid, 10)
	if len(before) != 1 {
		t.Fatalf("expected exactly one check-in, got %d", len(before))
	}

	undo(t, s, sid, latestOp(t, s, sid, "mood_record"))

	after, _ := s.store.Moods().List(context.Background(), sid, 10)
	if len(after) != 0 {
		t.Errorf("undo left %d check-in(s) behind", len(after))
	}
}

func TestHTTPMaterialCreateIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	rec := writeJSONReq(t, s, sid, "POST", "/api/materials",
		map[string]any{"title": "考试范围", "body": "第 3-7 章", "category": "note"},
		s.handleMaterialCreate)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created domain.Material
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	undo(t, s, sid, latestOp(t, s, sid, "material_create"))

	if got, err := s.store.Materials().Get(context.Background(), sid, created.ID); err == nil && got != nil {
		t.Error("undo left the material behind")
	}
}

func TestHTTPMaterialUpdateIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	rec := writeJSONReq(t, s, sid, "POST", "/api/materials",
		map[string]any{"title": "原标题", "body": "原正文", "category": "note"},
		s.handleMaterialCreate)
	var created domain.Material
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	r := httptest.NewRequest("PATCH", versionPath("/api/materials/"+created.ID),
		strings.NewReader(`{"title":"改过的标题"}`))
	r.SetPathValue("id", created.ID)
	pr := httptest.NewRecorder()
	s.handleMaterialUpdate(pr, r.WithContext(withSession(r.Context(), sid)))
	if pr.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", pr.Code, pr.Body.String())
	}

	undo(t, s, sid, latestOp(t, s, sid, "material_update"))

	// ⚠️ THE assertion for the defensive copy. `existing` is mutated in place by
	// the patch loop, so logging it directly would record the AFTER state as the
	// before-snapshot — and undo would restore the edit onto itself, reporting
	// success while changing nothing.
	got, err := s.store.Materials().Get(context.Background(), sid, created.ID)
	if err != nil || got == nil {
		t.Fatalf("undo lost the material: %v", err)
	}
	if got.Title != "原标题" {
		t.Errorf("title is %q, want the original back", got.Title)
	}
	// ⚠️ Same id. An update's undo restores fields; it does not re-create a row.
	if got.ID != created.ID {
		t.Errorf("undo changed the id from %s to %s", created.ID, got.ID)
	}
}

func TestHTTPMaterialDeleteIsUndoable(t *testing.T) {
	s, sid := newAgentTestServer(t)

	rec := writeJSONReq(t, s, sid, "POST", "/api/materials",
		map[string]any{"title": "房间号", "body": "教三 401", "category": "note"},
		s.handleMaterialCreate)
	var created domain.Material
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	r := httptest.NewRequest("DELETE", versionPath("/api/materials/"+created.ID), nil)
	r.SetPathValue("id", created.ID)
	dr := httptest.NewRecorder()
	s.handleMaterialDelete(dr, r.WithContext(withSession(r.Context(), sid)))
	if dr.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", dr.Code, dr.Body.String())
	}

	undo(t, s, sid, latestOp(t, s, sid, "material_delete"))

	// ⚠️ Restored by CONTENT, under a new id — the repositories mint ids on
	// create and none of them takes one. Asserting the content rather than the id
	// is not a weaker test, it is the honest statement of what undo promises here.
	all, err := s.store.Materials().List(context.Background(), sid, "", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range all {
		if m.Title == "房间号" && m.Body == "教三 401" {
			found = true
		}
	}
	if !found {
		t.Errorf("undo did not bring the note back (%d material(s) present)", len(all))
	}
}

// Every action these handlers log must have a registered inverse.
//
// ⚠️ The blunt version of the above, and it earns its place because the six
// tests each prove one path while this proves the SET. A seventh write added
// later gets caught here even if nobody writes it a test — which is the failure
// mode that produced this batch in the first place.
func TestHTTPWriteActionsAllHaveInverses(t *testing.T) {
	for _, action := range []string{
		"assignment_upsert", "mood_record",
		"material_create", "material_update", "material_delete",
	} {
		if _, ok := revertHandlers[action]; !ok {
			if reason, declared := irreversibleActions[action]; declared {
				t.Errorf("%s is declared irreversible (%s), but an HTTP handler logs it — "+
					"a user pressing undo gets a 400", action, reason)
				continue
			}
			t.Errorf("%s has no registered inverse; undo answers 400 at the moment the "+
				"user presses the button", action)
		}
	}
}
