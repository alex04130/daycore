package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("long-term memory", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/memory", s.handleMemoryList)
		mux.HandleFunc("POST /api/memory", s.handleMemoryAdd)
		mux.HandleFunc("DELETE /api/memory", s.handleMemoryClear)
		mux.HandleFunc("DELETE /api/memory/{id}", s.handleMemoryDelete)
		mux.HandleFunc("GET /api/import/history", s.handleImportHistory)
	})
}

// maxFactLen keeps single facts prompt-sized.
const maxFactLen = 500

var validFactSources = map[string]bool{"chat": true, "user": true, "import": true}

// GET /api/memory — every long-term fact the app has remembered.
func (s *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	facts, err := s.store.Memory().ListFacts(r.Context(), sid)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取记忆失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"facts": facts})
}

// POST /api/memory — remember one fact (from chat <memory_update> or the user).
func (s *Server) handleMemoryAdd(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Fact   string `json:"fact"`
		Source string `json:"source"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Fact) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 fact")
		return
	}
	fact := strings.TrimSpace(body.Fact)
	if len(fact) > maxFactLen {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "fact 过长")
		return
	}
	source := orDefault(body.Source, "chat")
	if !validFactSources[source] {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "source 必须是 chat/user/import")
		return
	}
	created, err := s.store.Memory().AddFact(r.Context(), &domain.MemoryFact{
		SessionID: sid, Fact: fact, Source: source,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "记忆保存失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "memory_add", TargetID: created.ID,
		Summary: created.Fact, Detail: marshalCompact(created),
	})
	s.writeJSON(w, http.StatusOK, created)
}

// DELETE /api/memory/{id} — forget one fact.
func (s *Server) handleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	// Snapshot the fact before deleting so the operation is revertible.
	var prev *domain.MemoryFact
	if facts, err := s.store.Memory().ListFacts(r.Context(), sid); err == nil {
		for i := range facts {
			if facts[i].ID == id {
				prev = &facts[i]
				break
			}
		}
	}
	err := s.store.Memory().DeleteFact(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "fact_not_found", "没有这条记忆")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "记忆删除失败")
		return
	}
	summary := id
	if prev != nil {
		summary = prev.Fact
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "memory_delete", TargetID: id,
		Summary: summary, Detail: marshalCompact(map[string]any{"before": prev}),
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/memory — forget everything (privacy hard requirement).
func (s *Server) handleMemoryClear(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	// Snapshot all facts before clearing so the operation is revertible.
	var prev []domain.MemoryFact
	if facts, err := s.store.Memory().ListFacts(r.Context(), sid); err == nil {
		prev = facts
	}
	n, err := s.store.Memory().ClearFacts(r.Context(), sid)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "记忆清空失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "memory_clear",
		Summary: fmt.Sprintf("cleared %d facts", n),
		Detail:  marshalCompact(map[string]any{"before": prev}),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": n})
}

// GET /api/import/history — append-only archive of every upload (no payloads).
func (s *Server) handleImportHistory(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	items, err := s.store.Memory().ListImports(r.Context(), sid, 50)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取导入历史失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"imports": items})
}

// recordImport archives one upload append-only; failures only log (an archive
// miss must never fail the import itself).
func (s *Server) recordImport(ctx context.Context, sid, source string, items int, summary, payload string) {
	_, err := s.store.Memory().AddImport(ctx, &domain.ImportRecord{
		SessionID: sid, Source: source, Items: items, Summary: summary, Payload: payload,
	})
	if err != nil {
		s.log.Error("import archive", "err", err, "source", source)
	}
}

// memoryFactsContext renders facts as compact JSON [{id, fact}] for prompts
// ("[]" when none or on failure).
func (s *Server) memoryFactsContext(ctx context.Context, sid string) string {
	if sid == "" {
		return "[]"
	}
	facts, err := s.store.Memory().ListFacts(ctx, sid)
	if err != nil || len(facts) == 0 {
		return "[]"
	}
	out := make([]map[string]string, 0, len(facts))
	for _, f := range facts {
		out = append(out, map[string]string{"id": f.ID, "fact": f.Fact})
	}
	return marshalCompact(out)
}
