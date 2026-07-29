package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"

	"github.com/google/uuid"
)

func init() {
	registerRoutes("ai", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/ai/companion/async", s.handleAICompanionAsync)
	})
}

// recordingSink captures agent frames in memory for persistence into the
// placeholder message's toolEvents (same frame objects the SSE protocol emits,
// so clients replay them identically). Deltas/reasoning are dropped — the final
// text comes from runCompanionAgent's return value. When a decision card
// arrives, onCard flushes the events immediately so a polling client can see
// and answer the card (POST /api/decisions/{id}/respond) while the agent is
// still blocked waiting.
type recordingSink struct {
	mu     sync.Mutex
	events []map[string]any
	onCard func(eventsJSON string)
}

func (rs *recordingSink) send(v map[string]any) {
	t, _ := v["type"].(string)
	switch t {
	case "tool_start", "tool_result", "decision_card":
	default:
		return
	}
	rs.mu.Lock()
	rs.events = append(rs.events, v)
	snapshot := ""
	if t == "decision_card" && rs.onCard != nil {
		if b, err := json.Marshal(rs.events); err == nil {
			snapshot = string(b)
		}
	}
	rs.mu.Unlock()
	if snapshot != "" {
		rs.onCard(snapshot)
	}
}

func (rs *recordingSink) ping() {}

func (rs *recordingSink) fail(code, message string) {
	rs.mu.Lock()
	rs.events = append(rs.events, map[string]any{"type": "error", "code": code, "message": message})
	rs.mu.Unlock()
}

// eventsJSON returns the recorded frames as a JSON array ("" when none).
func (rs *recordingSink) eventsJSON() string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.events) == 0 {
		return ""
	}
	b, err := json.Marshal(rs.events)
	if err != nil {
		return ""
	}
	return string(b)
}

// Polling clients need more reaction time on decision cards than a live stream.
func (rs *recordingSink) decisionWait() time.Duration { return decisionTimeoutAsync }

// POST /api/ai/companion/async — fire-and-forget companion turn. Persists the
// user message plus a pending assistant placeholder, returns 202 immediately,
// and runs the agent in a background goroutine detached from the client
// connection (context.Background(), the channel_agent.go pattern). Clients
// poll GET /api/chat/messages/{id} (or the thread's message list) until status
// leaves "pending". threadId is required — without a thread there is nowhere
// to pick up the result.
func (s *Server) handleAICompanionAsync(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Message       string `json:"message"`
		Timezone      string `json:"timezone"`
		AssistantName string `json:"assistantName"`
		ThreadID      string `json:"threadId"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Message) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 message")
		return
	}
	if body.ThreadID == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "异步模式必须提供 threadId（结果写回该线程）")
		return
	}
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	// Ownership: the thread must belong to this session (messages carry sid
	// too, but failing early beats a placeholder nobody can ever read).
	owned := false
	if threads, err := s.store.Chats().ListThreads(r.Context(), sid); err == nil {
		for _, th := range threads {
			if th.ID == body.ThreadID {
				owned = true
				break
			}
		}
	}
	if !owned {
		s.writeErr(w, http.StatusNotFound, "thread_not_found", "会话不存在")
		return
	}
	s.decisions.cancelForSession(sid) // a new message supersedes any pending card

	// Pre-generate both IDs: mongostore's AppendMessages does not write
	// generated IDs back to the caller's slice.
	userMsgID := uuid.NewString()
	placeholderID := uuid.NewString()
	err := s.store.Chats().AppendMessages(r.Context(), []domain.ChatMessage{
		{ID: userMsgID, ThreadID: body.ThreadID, SessionID: sid, Role: domain.RoleUser, Content: body.Message},
		{ID: placeholderID, ThreadID: body.ThreadID, SessionID: sid, Role: domain.RoleAssistant, Status: domain.MsgStatusPending},
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "消息保存失败")
		return
	}

	locale := s.requestLocale(r)
	name := orDefault(body.AssistantName, "Leo")
	clientIP := s.clientIP(r) // capture now — r dies with the request
	threadID := body.ThreadID
	userMsg := body.Message
	tz := body.Timezone

	s.GoTracked(func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.AIRequestTimeout)
		defer cancel()
		finalize := func(content, toolEvents, status string) {
			upd := domain.ChatMessageUpdate{Content: &content, Status: &status}
			if toolEvents != "" {
				upd.ToolEvents = &toolEvents
			}
			if err := s.store.Chats().UpdateMessage(context.WithoutCancel(ctx), sid, placeholderID, upd); err != nil {
				s.log.Error("async companion finalize", "err", err, "msg", placeholderID)
			}
		}
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("async companion panic", "err", rec)
				finalize(asyncErrorText(locale), "", domain.MsgStatusError)
			}
		}()

		// Exclude the rows we just wrote — the user turn is appended manually
		// below (via buildCompanionMessages) and the placeholder is empty.
		exclude := map[string]bool{userMsgID: true, placeholderID: true}
		messages, err := s.buildCompanionMessages(ctx, sid, threadID, locale, tz, name, userMsg, exclude)
		if err != nil {
			s.log.Error("async companion prompt", "err", err)
			finalize(asyncErrorText(locale), "", domain.MsgStatusError)
			return
		}
		sink := &recordingSink{}
		sink.onCard = func(eventsJSON string) {
			// Status stays pending; the client sees the card in toolEvents and
			// answers through the existing respond endpoint.
			_ = s.store.Chats().UpdateMessage(context.WithoutCancel(ctx), sid, placeholderID,
				domain.ChatMessageUpdate{ToolEvents: &eventsJSON})
		}
		req := (&http.Request{RemoteAddr: clientIP, Header: http.Header{}}).WithContext(ctx)
		answer := s.runCompanionAgent(ctx, sink, req, s.catalog.DefaultChat(), sid, locale, tz, messages, true)
		if strings.TrimSpace(answer) == "" {
			finalize(asyncErrorText(locale), sink.eventsJSON(), domain.MsgStatusError)
			return
		}
		finalize(answer, sink.eventsJSON(), domain.MsgStatusDone)
	})

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"threadId":      threadID,
		"userMessageId": userMsgID,
		"messageId":     placeholderID,
		"status":        domain.MsgStatusPending,
	})
}

var asyncErrorMsg = i18n.Reg("companion.asyncError", i18n.Text{
	"zh-CN": "抱歉，这条消息处理失败了，请重试。",
	"en-US": "Sorry, this message failed to process — please try again.",
})

func asyncErrorText(locale string) string { return i18n.T(asyncErrorMsg, locale) }
