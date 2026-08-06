package server

import (
	"errors"
	"net/http"
	"strconv"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("chat threads", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/chat/threads", s.handleChatListThreads)
		mux.HandleFunc("POST /api/chat/threads", s.handleChatCreateThread)
		mux.HandleFunc("PATCH /api/chat/threads/{id}", s.handleChatUpdateThread)
		mux.HandleFunc("DELETE /api/chat/threads/{id}", s.handleChatDeleteThread)
		mux.HandleFunc("GET /api/chat/threads/{id}/messages", s.handleChatListMessages)
		mux.HandleFunc("DELETE /api/chat/threads/{id}/messages", s.handleChatClearMessages)
		mux.HandleFunc("GET /api/chat/messages/{id}", s.handleChatGetMessage)
	})
}

// ─── chat thread handlers ────────────────────────────────────────────────────

// GET /api/chat/threads — list all threads for the session.
func (s *Server) handleChatListThreads(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	threads, err := s.store.Chats().ListThreads(r.Context(), sid)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatListThreads.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatCreateThread.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.chatUpdateThread.bad_request")
		return
	}
	upd := domain.ChatThreadUpdate{Title: body.Title, Archived: body.Archived}
	thread, err := s.store.Chats().UpdateThread(r.Context(), sid, id, upd)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "thread_not_found", "err.chatUpdateThread.thread_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatUpdateThread.internal")
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
	s.dropThreadAttachments(r.Context(), sid, id)
	if err := s.store.Chats().DeleteThreadMessages(r.Context(), sid, id); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatClearMessages.internal")
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
	// Before the messages go: after that there is nothing left that knows which
	// blobs this conversation owned, and the file bus cannot be asked.
	s.dropThreadAttachments(r.Context(), sid, id)
	if err := s.store.Chats().DeleteThread(r.Context(), sid, id); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatDeleteThread.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "message_not_found", "err.chatGetMessage.message_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatGetMessage.internal")
		return
	}
	one := []domain.ChatMessage{*msg}
	s.hydrateAttachments(r.Context(), sid, one)
	s.writeJSON(w, http.StatusOK, one[0])
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
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.chatListMessages.bad_request")
			return
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	msgs, err := s.store.Chats().ListMessages(r.Context(), threadID, sid, before, limit)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.chatListMessages.internal")
		return
	}
	if msgs == nil {
		msgs = []domain.ChatMessage{}
	}
	s.hydrateAttachments(r.Context(), sid, msgs)
	s.writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}
