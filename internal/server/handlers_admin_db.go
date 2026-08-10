package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/version"
)

func init() {
	registerRoutes("admin (stats, users, DB)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/db/tables", s.handleAdminDBTables)
		mux.HandleFunc("GET /api/admin/db/table/{name}", s.handleAdminDBTableBrowse)
		mux.HandleFunc("DELETE /api/admin/db/table/{name}/{id}", s.handleAdminDBTableDelete)
		mux.HandleFunc("GET /api/admin/db/export", s.handleAdminDBExport)
		mux.HandleFunc("POST /api/admin/db/import", s.handleAdminDBImport)
		mux.HandleFunc("GET /api/admin/db/backup", s.handleAdminDBBackup)
	})
}

// The database browser.
//
// # The route-level permission is the WEAKER of the two, and the handler raises it
//
// `GET /api/admin/db/table/{name}` cannot tell from its pattern whether {name}
// is operation_logs or chat_messages, so routePermissions marks it
// db.operational and every handler here re-asks with the catalogue's class in
// hand. This is the one place in the codebase where a handler tightens what the
// gate allowed — see admin_gate.go, which explains why tightening is fine and
// standing in for the gate is not.
//
// It is also what makes the author's decision real: "chat_messages /
// mood_checkins / memory_facts 也作为权限可以分配，这样就是运维的活". A single
// db.browse would have left that sentence with nowhere to land.
//
// # Boundary: no filtering, no sorting, no search
//
// Deliberately. Every one of those means a caller-supplied identifier or
// predicate reaching the query, and domain.Tables exists precisely so that
// never happens. Somebody who needs to find one row by its content has a
// database client; somebody who does not should not be handed one through a web
// page. What the browser answers is "what is in here, and how much of it".
var (
	keyAdminDBUnknownTable = i18n.Reg("admin.db.unknown_table", i18n.Text{
		"zh-CN": "没有这张表",
		"en-US": "No such table",
	})
	keyAdminDBNotDeletable = i18n.Reg("admin.db.not_deletable", i18n.Text{
		"zh-CN": "这张表不能从这里删行：",
		"en-US": "Rows cannot be deleted from this table here: ",
	})
	keyAdminDBRowGone = i18n.Reg("admin.db.row_gone", i18n.Text{
		"zh-CN": "没有这一行，可能已经被删了",
		"en-US": "No such row — it may already be gone",
	})
	keyAdminDBForbidden = i18n.Reg("admin.db.forbidden_table", i18n.Text{
		"zh-CN": "这张表装的是用户自己写下的内容，要单独的权限才能看",
		"en-US": "This table holds what people wrote; reading it needs its own permission",
	})
	keyAdminDBDegraded = i18n.Reg("admin.db.degraded", i18n.Text{
		"zh-CN": "数据库不可用，这一屏读不到东西",
		"en-US": "The database is unavailable, so this screen has nothing to read",
	})
	keyAdminDBInternal = i18n.Reg("admin.db.internal", i18n.Text{
		"zh-CN": "读表失败",
		"en-US": "Could not read the table",
	})
	keyAdminDBImportOff = i18n.Reg("admin.db.import_unavailable", i18n.Text{
		"zh-CN": "这个版本没有导入。导入要覆盖正在被读写的表，而这需要事务 —— 存储层没有事务，半个导入会留下一个既不是旧数据也不是新数据的库。恢复请用 daycore install 起一个新部署再灌进去。",
		"en-US": "This build has no import. Importing means overwriting tables a live process is reading and writing, which needs transactions the storage layer does not have — a half-applied import leaves a database that is neither the old data nor the new. To restore, stand up a fresh deployment with `daycore install` and load it there.",
	})
	keyAdminDBBackupOff = i18n.Reg("admin.db.backup_unavailable", i18n.Text{
		"zh-CN": "这里不提供数据库文件下载。一致的快照只有引擎自己的工具做得出来：sqlite3 .backup / pg_dump / mysqldump / mongodump。想要一份能看的内容清单，用旁边的导出 JSON。",
		"en-US": "No database file download here. Only the engine's own tool can take a consistent snapshot: sqlite3 .backup / pg_dump / mysqldump / mongodump. For a readable inventory, use the JSON export beside this.",
	})
)

// permForTable maps the catalogue's class onto the two browse permissions.
//
// ⚠️ db.user_content is registered ONLY because this function is what consults
// it. Until this handler existed the constant was deliberately unregistered — a
// console switch that grants nothing reads like protection and is worse than no
// switch at all. If this split is ever removed, unregister it again.
func permForTable(t domain.Table) string {
	if t.Class == domain.TableUserContent {
		return PermDBUserContent
	}
	return PermDBOperational
}

// lookupTable resolves {name}, checks the caller may see that class, and writes
// the refusal itself.
func (s *Server) lookupTable(w http.ResponseWriter, r *http.Request) (domain.Table, bool) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminDBDegraded, locale))
		return domain.Table{}, false
	}
	t, ok := domain.TableByName(r.PathValue("name"))
	if !ok {
		s.writeErr(w, http.StatusNotFound, "unknown_table", i18n.T(keyAdminDBUnknownTable, locale))
		return domain.Table{}, false
	}
	if !s.authorize(r, permForTable(t)) {
		s.writeErr(w, http.StatusForbidden, "forbidden", i18n.T(keyAdminDBForbidden, locale))
		return domain.Table{}, false
	}
	return t, true
}

type adminTableCard struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	Rows  int64  `json:"rows"`
	// Readable says whether THIS caller may open it. The card is still listed
	// when false, with its row count, because "how big is chat_messages" is an
	// operational fact — and hiding a table's existence from somebody who can
	// see every other one just makes the console look broken.
	Readable  bool   `json:"readable"`
	Deletable bool   `json:"deletable"`
	WhyNot    string `json:"whyNot,omitempty"`
}

// GET /api/admin/db/tables — every table, its class, and how many rows.
//
// This was twenty table names written out by hand in this file. By the time
// anybody looked it was missing seventeen of the thirty-seven that exist —
// every table added after it was written, including the six the multi-instance
// work introduced. A hand-maintained copy of a generated thing is a copy that
// is wrong; the catalogue it reads now has a test holding it to the schema.
func (s *Server) handleAdminDBTables(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminDBDegraded, locale))
		return
	}
	ctx := r.Context()
	b := s.store.Browser()
	canDelete := s.authorize(r, PermDBDeleteRow)
	out := make([]adminTableCard, 0, len(domain.Tables))
	for _, t := range domain.Tables {
		card := adminTableCard{
			Name:      t.Name,
			Class:     string(t.Class),
			Readable:  s.authorize(r, permForTable(t)),
			Deletable: t.Deletable() && canDelete,
			WhyNot:    t.NotDeletableWhy,
		}
		if n, err := b.CountRows(ctx, t.Name); err == nil {
			card.Rows = n
		} else {
			// One uncountable table must not take the whole screen down — the
			// count is the least important thing on the card, and the operator
			// is probably here because something is already wrong.
			card.Rows = -1
			s.log.Warn("could not count a table for the console", "table", t.Name, "err", err)
		}
		out = append(out, card)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"tables": out})
}

// GET /api/admin/db/table/{name}?limit=&offset=
func (s *Server) handleAdminDBTableBrowse(w http.ResponseWriter, r *http.Request) {
	t, ok := s.lookupTable(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	page, err := s.store.Browser().BrowseRows(r.Context(), t.Name, limit, offset)
	if err != nil {
		s.log.Error("db browse", "table", t.Name, "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminDBInternal, s.requestLocale(r)))
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"table":     t.Name,
		"class":     string(t.Class),
		"columns":   page.Columns,
		"rows":      page.Rows,
		"total":     page.Total,
		"limit":     domain.ListLimit(limit, domain.BrowseListDefault, domain.BrowseListMax),
		"offset":    offset,
		"deletable": t.Deletable(),
		"whyNot":    t.NotDeletableWhy,
		// Named, so the console can label the column rather than render a
		// stripe of identical placeholder text with no explanation.
		"redacted": t.Redact,
	})
}

// DELETE /api/admin/db/table/{name}/{id}
//
// ⚠️ Bypasses every business rule and writes nothing to the undo ledger. That
// is exactly what db.delete_row's damage line says, and it is accurate: there
// is no compensation to record, because the browser does not know what the row
// meant.
func (s *Server) handleAdminDBTableDelete(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	// TWO permissions are required and NEITHER is checked on this line.
	//
	//	db.delete_row      by adminGate, from routePermissions — reaching this
	//	                   function at all means the caller holds it
	//	the table's class  by lookupTable, just below
	//
	// A third check here would be dead code, and this file's own test proved it:
	// deleting the handler-level `authorize(r, PermDBDeleteRow)` that used to sit
	// here left every assertion green, because the gate had already refused
	// everybody it would have refused. Dead defensive code is worse than none —
	// it makes the next reader think the gate is optional.
	t, ok := s.lookupTable(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	gone, err := s.store.Browser().DeleteRow(r.Context(), t.Name, id)
	switch {
	case errors.Is(err, domain.ErrUnsupported):
		s.writeErr(w, http.StatusConflict, "not_deletable",
			i18n.T(keyAdminDBNotDeletable, locale)+t.NotDeletableWhy)
		return
	case err != nil:
		s.log.Error("db delete row", "table", t.Name, "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminDBInternal, locale))
		return
	case !gone:
		s.writeErr(w, http.StatusNotFound, "row_not_found", i18n.T(keyAdminDBRowGone, locale))
		return
	}
	// Loudly, in the process log. The one action in this console with no undo
	// and no ledger entry should at least leave a line somewhere.
	s.log.Warn("a row was deleted from the console", "table", t.Name, "id", id)
	s.writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "table": t.Name})
}

// GET /api/admin/db/export — the catalogue, as JSON.
//
// # What it refuses, and why that is the design rather than a limitation
//
// It exports the OPERATIONAL half and skips the user-content half unless the
// caller also holds db.user_content. Anything else would make db.export a way
// around the split the rest of this file maintains: one click, and somebody
// with only db.operational has every conversation in the deployment on a disk.
//
// Redacted columns stay redacted. An export carrying password hashes turns one
// stolen laptop into every account.
//
// ⚠️ A JSON snapshot, NOT a backup. There are no transactions anywhere in the
// storage layer, so tables are read one after another and a write landing
// between two of them appears in one and not the other. It is for "what is in
// this deployment" and for moving a small self-hosted instance — the note
// travels inside the file, because the file outlives the screen that made it.
func (s *Server) handleAdminDBExport(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	if s.store == nil {
		s.writeErr(w, http.StatusServiceUnavailable, "degraded", i18n.T(keyAdminDBDegraded, locale))
		return
	}
	ctx := r.Context()
	b := s.store.Browser()
	includeUserContent := s.authorize(r, PermDBUserContent)

	tables := map[string]any{}
	skipped := []string{}
	for _, t := range domain.Tables {
		if t.Class == domain.TableUserContent && !includeUserContent {
			skipped = append(skipped, t.Name)
			continue
		}
		// Paged rather than one query per table, so a large table is not
		// materialised at full size twice (once by the driver, once as JSON).
		rows := []map[string]any{}
		for offset := 0; ; offset += domain.BrowseListMax {
			page, err := b.BrowseRows(ctx, t.Name, domain.BrowseListMax, offset)
			if err != nil {
				s.log.Error("db export", "table", t.Name, "err", err)
				s.writeErr(w, http.StatusInternalServerError, "internal", i18n.T(keyAdminDBInternal, locale))
				return
			}
			for _, row := range page.Rows {
				m := make(map[string]any, len(page.Columns))
				for i, c := range page.Columns {
					if i < len(row) {
						m[c] = row[i]
					}
				}
				rows = append(rows, m)
			}
			if len(page.Rows) < domain.BrowseListMax {
				break
			}
		}
		tables[t.Name] = rows
	}

	w.Header().Set("Content-Disposition", `attachment; filename="daycore-export.json"`)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"exportedAt": time.Now().UTC().Format(time.RFC3339),
		"version":    version.Version,
		"note": "JSON snapshot, not a consistent backup: tables are read one after another with no " +
			"transaction, and redacted columns are absent by design. For a real backup use the engine's own tool.",
		"skippedUserTables": skipped,
		"tables":            tables,
	})
}

// POST /api/admin/db/import — refused, in words.
//
// ⚠️ NOT a stub awaiting an implementation. It used to answer `{"ok": true}`
// while doing nothing at all, which is the worst answer available: somebody
// restoring a deployment gets a success message and an empty database, and
// finds out later.
//
// Refused rather than written because importing means overwriting tables a live
// process is reading and writing, and there are no transactions anywhere in
// this storage layer. A half-applied import leaves a database in a state
// neither the old data nor the new one describes. The honest path is a fresh
// deployment, which `daycore install` already builds.
func (s *Server) handleAdminDBImport(w http.ResponseWriter, r *http.Request) {
	// Drain a bounded amount of the body first, so a client that started
	// streaming a large file gets a clean refusal rather than a connection reset
	// mid-upload — which reads as a network fault rather than as an answer.
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(new(any))
	s.writeErr(w, http.StatusNotImplemented, "import_unavailable",
		i18n.T(keyAdminDBImportOff, s.requestLocale(r)))
}

// GET /api/admin/db/backup — refused on every engine, with the tool to use.
//
// The prototype has a "备份 .db" button and SQLite could technically serve its
// file. It does not, and the reason is that serving it would be a LIE on the one
// occasion it matters: a live SQLite database copied byte-for-byte while a
// write is in flight is a corrupt file, and the WAL beside it holds the
// difference. `sqlite3 .backup` exists because that is a real problem.
//
// So rather than four different half-answers, one honest one that names the
// right tool per engine. ⚠️ If this is ever implemented for SQLite it must go
// through the backup API, not io.Copy.
func (s *Server) handleAdminDBBackup(w http.ResponseWriter, r *http.Request) {
	s.writeErr(w, http.StatusNotImplemented, "backup_unavailable",
		i18n.T(keyAdminDBBackupOff, s.requestLocale(r)))
}
