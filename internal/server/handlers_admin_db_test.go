package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// The per-table permission split is enforced at the handler, not the route.
//
// This is the assertion the whole catalogue exists for. The route declares
// db.operational because a pattern cannot tell operation_logs from
// chat_messages, so if the handler ever stops re-asking, **db.operational
// silently becomes db.everything** — and the author's decision that user content
// is separately assignable becomes decoration. Nothing else in the suite would
// notice: the route gate still passes, the browser still works, and the only
// symptom is that a support engineer can read everybody's diary.
func TestTheDatabaseBrowserSplitsUserContentFromOperational(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "ops", []string{PermDBOperational})
	makeAdminUser(t, s, "deep", []string{PermDBOperational, PermDBUserContent})
	ops := withAdminCookie(t, s, "ops")
	deep := withAdminCookie(t, s, "deep")

	// An operational table: both may open it.
	for name, as := range map[string]func(*http.Request){"ops": ops, "deep": deep} {
		if rec := adminReq(t, s, http.MethodGet, "/api/admin/db/table/operation_logs", "", as); rec.Code != http.StatusOK {
			t.Errorf("%s could not browse operation_logs: %d %s", name, rec.Code, rec.Body)
		}
	}
	// A user-content table: only the second.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/db/table/chat_messages", "", ops); rec.Code != http.StatusForbidden {
		t.Errorf("db.operational alone opened chat_messages: %d — the split is not being enforced", rec.Code)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/db/table/chat_messages", "", deep); rec.Code != http.StatusOK {
		t.Errorf("db.user_content did not open chat_messages: %d %s", rec.Code, rec.Body)
	}
}

// Export obeys the same split. It is the shortest way around it if it does not:
// one click and somebody with only db.operational has every conversation.
func TestExportSkipsUserContentWithoutThePermission(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "exporter", []string{PermDBExport, PermDBOperational})
	makeAdminUser(t, s, "everything", []string{PermDBExport, PermDBOperational, PermDBUserContent})

	read := func(who string) (map[string]any, []string) {
		t.Helper()
		rec := adminReq(t, s, http.MethodGet, "/api/admin/db/export", "", withAdminCookie(t, s, who))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: export answered %d %s", who, rec.Code, rec.Body)
		}
		var body struct {
			Tables            map[string]any `json:"tables"`
			SkippedUserTables []string       `json:"skippedUserTables"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Tables, body.SkippedUserTables
	}

	tables, skipped := read("exporter")
	if len(skipped) == 0 {
		t.Fatal("nothing was skipped for a caller without db.user_content — the export walks around the split")
	}
	for _, name := range skipped {
		if _, present := tables[name]; present {
			t.Errorf("%s is listed as skipped and is in the payload anyway", name)
		}
	}
	if _, present := tables["chat_messages"]; present {
		t.Error("chat_messages was exported to somebody holding only db.operational")
	}
	if _, present := tables["operation_logs"]; !present {
		t.Error("operation_logs was not exported to somebody who may read it")
	}

	full, fullSkipped := read("everything")
	if len(fullSkipped) != 0 {
		t.Errorf("a caller with db.user_content still had %v skipped", fullSkipped)
	}
	if _, present := full["chat_messages"]; !present {
		t.Error("chat_messages was not exported to somebody who may read it")
	}
}

// Redacted columns stay redacted through the HTTP layer too, not only in the
// store. The store's conformance case proves the repository redacts; this
// proves nothing above it puts the value back.
func TestTheBrowserNeverServesABearerToken(t *testing.T) {
	s := adminServer(t)
	if _, err := s.store.Sessions().GetOrCreate(context.Background(), "s-http"); err != nil {
		t.Fatal(err)
	}
	rec := adminReq(t, s, http.MethodGet, "/api/admin/db/table/sessions", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("browse sessions: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Columns  []string `json:"columns"`
		Rows     [][]any  `json:"rows"`
		Redacted []string `json:"redacted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Rows) == 0 {
		t.Fatal("no rows came back; this assertion would prove nothing")
	}
	at := -1
	for i, c := range body.Columns {
		if c == "import_token" {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("the sessions page has no import_token column — redaction is not being exercised")
	}
	for _, row := range body.Rows {
		if row[at] != domain.RedactedValue {
			t.Errorf("import_token was served as %v", row[at])
		}
	}
	// And it is NAMED, so the console can label the column rather than showing
	// a stripe of identical placeholder text with no explanation.
	if len(body.Redacted) == 0 {
		t.Error("the response does not say which columns were redacted")
	}
}

// A table nobody catalogued is a 404, and a name that is not a table at all is
// the same 404 rather than anything the query layer had to deal with.
func TestTheBrowserOnlyServesTheCatalogue(t *testing.T) {
	s := adminServer(t)
	root := withRootHeader(s)
	for _, name := range []string{"sqlite_master", "users%3B%20DROP%20TABLE%20users", "nope"} {
		rec := adminReq(t, s, http.MethodGet, "/api/admin/db/table/"+name, "", root)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%q answered %d, want 404", name, rec.Code)
		}
	}
	// And the table list is the catalogue, not a hand-written subset. It was a
	// hand-written subset once, and it had drifted to missing seventeen tables.
	rec := adminReq(t, s, http.MethodGet, "/api/admin/db/tables", "", root)
	if rec.Code != http.StatusOK {
		t.Fatalf("table list: %d %s", rec.Code, rec.Body)
	}
	var listed struct {
		Tables []struct {
			Name string `json:"name"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tables) != len(domain.Tables) {
		t.Errorf("the console was shown %d tables and the catalogue has %d", len(listed.Tables), len(domain.Tables))
	}
}

// Deleting a row needs BOTH the class permission and db.delete_row, and a table
// with no single-row key refuses in a way that says the row is still there.
func TestDeletingARowNeedsBothPermissions(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	l := aiLogRow(t, s, "s-x")
	makeAdminUser(t, s, "reader", []string{PermDBOperational})
	makeAdminUser(t, s, "destroyer", []string{PermDBOperational, PermDBDeleteRow})

	// Reading is not deleting.
	rec := adminReq(t, s, http.MethodDelete, "/api/admin/db/table/ai_call_logs/"+l, "", withAdminCookie(t, s, "reader"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("db.operational alone deleted a row: %d", rec.Code)
	}
	if n, _ := s.store.Browser().CountRows(ctx, "ai_call_logs"); n != 1 {
		t.Fatalf("the row went anyway; %d left", n)
	}

	rec = adminReq(t, s, http.MethodDelete, "/api/admin/db/table/ai_call_logs/"+l, "", withAdminCookie(t, s, "destroyer"))
	if rec.Code != http.StatusOK {
		t.Errorf("db.delete_row could not delete: %d %s", rec.Code, rec.Body)
	}
	if n, _ := s.store.Browser().CountRows(ctx, "ai_call_logs"); n != 0 {
		t.Errorf("the row is still there")
	}
	// Gone is 404, not a second success.
	rec = adminReq(t, s, http.MethodDelete, "/api/admin/db/table/ai_call_logs/"+l, "", withAdminCookie(t, s, "destroyer"))
	if rec.Code != http.StatusNotFound {
		t.Errorf("deleting an absent row answered %d, want 404", rec.Code)
	}

	// A composite-key table is 409 with an explanation, NOT 404 — 404 would tell
	// an operator the row is gone when it is still there.
	rec = adminReq(t, s, http.MethodDelete, "/api/admin/db/table/roles/support", "", withRootHeader(s))
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting from roles answered %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "成员") {
		t.Errorf("the refusal does not say why: %s", rec.Body)
	}
}

// Import and backup refuse in words rather than answering ok.
//
// Import in particular used to answer 200 {"ok":true} while doing nothing, so
// somebody restoring a deployment got a success message and an empty database.
func TestImportAndBackupRefuseHonestly(t *testing.T) {
	s := adminServer(t)
	root := withRootHeader(s)

	rec := adminReq(t, s, http.MethodPost, "/api/admin/db/import", `{"tables":{}}`, root)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("import answered %d, want 501 — a fake success here loses somebody's restore", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("import claimed success: %s", rec.Body)
	}
	// The refusal has to say what to do instead, or it is just a wall.
	if !strings.Contains(rec.Body.String(), "install") {
		t.Errorf("the import refusal does not name an alternative: %s", rec.Body)
	}

	rec = adminReq(t, s, http.MethodGet, "/api/admin/db/backup", "", root)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("backup answered %d, want 501", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pg_dump") {
		t.Errorf("the backup refusal does not name the tool to use: %s", rec.Body)
	}
}

func aiLogRow(t *testing.T, s *Server, sid string) string {
	t.Helper()
	l := &domain.AICallLog{SessionID: sid, Endpoint: "companion", Model: "m", Status: "ok"}
	if err := s.store.AILogs().Add(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	return l.ID
}
