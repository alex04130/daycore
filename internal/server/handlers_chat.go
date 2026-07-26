package server

import (
	"errors"
	"net/http"
	"strconv"

	"daycore/internal/domain"
)

// ─── chat thread handlers ────────────────────────────────────────────────────

// GET /api/chat/threads — list all threads for the session.
func (s *Server) handleChatListThreads(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	threads, err := s.store.Chats().ListThreads(r.Context(), sid)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取会话列表失败")
		return
	}

	// Auto-import from legacy companion_memory if no threads exist yet.
	if len(threads) == 0 {
		if mem, err := s.store.Companion().Get(r.Context(), sid); err == nil && len(mem.ConversationHistory) > 0 {
			thread, err := s.store.Chats().CreateThread(r.Context(), &domain.ChatThread{SessionID: sid, Title: "默认会话"})
			if err == nil {
				msgs := make([]domain.ChatMessage, 0, len(mem.ConversationHistory))
				for _, m := range mem.ConversationHistory {
					msgs = append(msgs, domain.ChatMessage{
						ThreadID: thread.ID, SessionID: sid,
						Role: string(m.Role), Content: m.Content,
					})
				}
				_ = s.store.Chats().AppendMessages(r.Context(), msgs)
				threads = []domain.ChatThread{*thread}
				// Consume the legacy source so deleting all threads later doesn't
				// resurrect it as a fresh import on the next list.
				_ = s.store.Companion().Upsert(r.Context(), sid, nil, mem.KeyFacts)
			}
		}
	}

	if threads == nil {
		threads = []domain.ChatThread{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

// POST /api/chat/threads — create a new thread.
func (s *Server) handleChatCreateThread(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	_ = s.readJSON(r, &body)
	title := body.Title
	if title == "" {
		title = "新的聊天"
	}
	// Truncate to 60 chars for display.
	if r := []rune(title); len(r) > 60 {
		title = string(r[:60])
	}
	thread, err := s.store.Chats().CreateThread(r.Context(), &domain.ChatThread{SessionID: sid, Title: title})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "创建会话失败")
		return
	}
	s.writeJSON(w, http.StatusOK, thread)
}

// PATCH /api/chat/threads/{id} — rename/archive a thread.
func (s *Server) handleChatUpdateThread(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Title    *string `json:"title"`
		Archived *bool   `json:"archived"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	upd := domain.ChatThreadUpdate{Title: body.Title, Archived: body.Archived}
	thread, err := s.store.Chats().UpdateThread(r.Context(), sid, id, upd)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "thread_not_found", "会话不存在")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "更新会话失败")
		return
	}
	s.writeJSON(w, http.StatusOK, thread)
}

// DELETE /api/chat/threads/{id}/messages — clear all messages in a thread.
func (s *Server) handleChatClearMessages(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := s.store.Chats().DeleteThreadMessages(r.Context(), sid, id); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "清空消息失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/chat/threads/{id} — delete a thread and all its messages.
func (s *Server) handleChatDeleteThread(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := s.store.Chats().DeleteThread(r.Context(), sid, id); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "删除会话失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/chat/messages/{id} — one message by id. Cheap polling target for
// async companion turns (status: pending → done/error).
func (s *Server) handleChatGetMessage(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	msg, err := s.store.Chats().GetMessage(r.Context(), sid, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "message_not_found", "消息不存在")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取消息失败")
		return
	}
	s.writeJSON(w, http.StatusOK, msg)
}

// GET /api/chat/threads/{id}/messages — list messages for a thread.
// Optional ?before=UnixMilli for cursor-based pagination.
func (s *Server) handleChatListMessages(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	threadID := r.PathValue("id")
	before := r.URL.Query().Get("before")
	if before != "" {
		if _, err := strconv.ParseInt(before, 10, 64); err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_request", "before 必须是毫秒时间戳")
			return
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	msgs, err := s.store.Chats().ListMessages(r.Context(), threadID, sid, before, limit)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取消息失败")
		return
	}
	if msgs == nil {
		msgs = []domain.ChatMessage{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}
