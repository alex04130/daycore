package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"daycore/internal/ai"
	"daycore/internal/i18n"

	"github.com/google/uuid"
)

// ─── SSE v2 sender ───────────────────────────────────────────────────────────
// One `data:` frame per event, discriminated by "type":
// delta / reasoning / tool_start / tool_result / decision_card / error / done.

type sseSender struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (s sseSender) send(v map[string]any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.w, "data: %s\n\n", b)
	_ = s.rc.Flush()
}

// ping keeps proxies from cutting the stream while a tool or decision waits.
func (s sseSender) ping() {
	fmt.Fprint(s.w, ": ping\n\n")
	_ = s.rc.Flush()
}

func (s sseSender) fail(code, message string) {
	s.send(map[string]any{"type": "error", "code": code, "message": message})
	s.send(map[string]any{"type": "done"})
}

// agentSink abstracts where agent frames go: the live SSE stream (sseSender),
// a discard sink (channel replies, channel_agent.go), or an in-memory recorder
// persisted into the placeholder message (async turns, recordingSink).
type agentSink interface {
	send(v map[string]any)
	ping()
	fail(code, message string)
}

// Decision-card wait budgets. A live SSE client sees the card instantly; a
// polling async client needs extra human reaction time.
const (
	decisionTimeoutSSE   = 45 * time.Second
	decisionTimeoutAsync = 90 * time.Second
)

// decisionWaiter is optionally implemented by sinks that need a non-default
// decision-card wait (recordingSink → decisionTimeoutAsync).
type decisionWaiter interface{ decisionWait() time.Duration }

// ─── decision cards ──────────────────────────────────────────────────────────
// propose_decision blocks its agent loop on a channel until the user answers
// via POST /api/decisions/{id}/respond, a timeout fires, or a newer companion
// message cancels it. At most one pending decision per session.

type decisionAnswer struct {
	Choice string
	Text   string
}

type pendingDecision struct {
	sid string
	ch  chan decisionAnswer
}

type decisionRegistry struct {
	mu      sync.Mutex
	pending map[string]*pendingDecision // decision id → waiter
}

func newDecisionRegistry() *decisionRegistry {
	return &decisionRegistry{pending: map[string]*pendingDecision{}}
}

// register cancels any pending decision for the session (one card at a time),
// then registers a new waiter.
func (r *decisionRegistry) register(sid, id string) chan decisionAnswer {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelLocked(sid)
	ch := make(chan decisionAnswer, 1)
	r.pending[id] = &pendingDecision{sid: sid, ch: ch}
	return ch
}

func (r *decisionRegistry) resolve(id, sid string, ans decisionAnswer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.pending[id]
	if p == nil || p.sid != sid {
		return false
	}
	p.ch <- ans
	delete(r.pending, id)
	return true
}

func (r *decisionRegistry) cancelForSession(sid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelLocked(sid)
}

func (r *decisionRegistry) cancelLocked(sid string) {
	for id, p := range r.pending {
		if p.sid == sid {
			select {
			case p.ch <- decisionAnswer{Choice: "cancelled"}:
			default:
			}
			delete(r.pending, id)
		}
	}
}

func (r *decisionRegistry) drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pending, id)
}

// POST /api/decisions/{id}/respond — resolve a pending decision card.
func (s *Server) handleDecisionRespond(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Choice string `json:"choice"`
		Text   string `json:"text"`
	}
	if err := s.readJSON(r, &body); err != nil || (body.Choice == "" && strings.TrimSpace(body.Text) == "") {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 choice 或 text")
		return
	}
	if !s.decisions.resolve(id, sid, decisionAnswer{Choice: body.Choice, Text: strings.TrimSpace(body.Text)}) {
		s.writeErr(w, http.StatusNotFound, "decision_not_found", "这张卡片已经过期了")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ─── the agent loop ──────────────────────────────────────────────────────────

// runCompanionAgent drives the multi-round tool loop, emitting frames into the
// given sink (the vision.go loop, rebuilt for streaming). messages must already
// contain the system prompt and the user's message. interactive=false strips
// propose_decision from the tool belt — for sinks whose client has no UI to
// answer a card (channel replies).
func (s *Server) runCompanionAgent(ctx context.Context, sink agentSink, r *http.Request, provider ai.AIProvider, sid, locale, tz string, messages []ai.Message, interactive bool) string {
	tools := companionToolDefs(provider.Capabilities(), interactive)
	var answer strings.Builder // accumulated assistant text, returned for persistence
	appendAnswer := func(text string) {
		if text == "" {
			return
		}
		if answer.Len() > 0 {
			answer.WriteString("\n")
		}
		answer.WriteString(text)
	}
	for round := 0; round < s.cfg.AgentMaxRounds; round++ {
		// Every extra round is one more model call — charge it to the same
		// per-IP budget as a fresh request so agent loops can't sidestep it.
		if round > 0 && !s.limiter.Allow(s.clientIP(r)) {
			break
		}
		resp, ok := s.streamRound(ctx, sink, provider, ai.ChatRequest{Messages: messages, Tools: tools, Temperature: 0.7})
		if !ok {
			return answer.String()
		}
		appendAnswer(resp.Content)
		if len(resp.ToolCalls) == 0 { // final answer already streamed as deltas
			sink.send(map[string]any{"type": "done"})
			return answer.String()
		}
		messages = append(messages, ai.Message{Role: ai.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			var args any
			_ = json.Unmarshal([]byte(call.Arguments), &args)
			sink.send(map[string]any{"type": "tool_start", "callId": call.ID, "tool": call.Name, "args": args})

			var res toolResult
			var cancelled bool
			if call.Name == "propose_decision" {
				res, cancelled = s.runProposeDecision(ctx, sink, sid, call)
			} else {
				res = s.runCompanionTool(ctx, sid, locale, tz, call)
			}

			out := map[string]any{"type": "tool_result", "callId": call.ID, "tool": call.Name, "ok": res.OK}
			if res.Summary != "" {
				out["summary"] = res.Summary
			}
			if res.Data != nil {
				out["data"] = res.Data
			}
			if res.OpID != "" {
				out["opId"] = res.OpID
			}
			if res.ErrMsg != "" {
				out["error"] = res.ErrMsg
			}
			sink.send(out)
			messages = append(messages, ai.Message{Role: ai.RoleTool, ToolCallID: call.ID, Name: call.Name, Content: res.JSON()})

			if cancelled { // the user moved on — this stream is stale
				sink.send(map[string]any{"type": "done"})
				return answer.String()
			}
		}
	}

	// Rounds (or the rate budget) exhausted: force a plain wrap-up, mirroring
	// vision.go's fallback — never end the stream without a spoken answer.
	messages = append(messages, ai.Message{Role: ai.RoleUser, Content: wrapUpNudge(locale)})
	resp, ok := s.streamRound(ctx, sink, provider, ai.ChatRequest{Messages: messages, Temperature: 0.7})
	if !ok {
		return answer.String() // streamRound already emitted error + done
	}
	if resp != nil {
		appendAnswer(resp.Content)
	}
	sink.send(map[string]any{"type": "done"})
	return answer.String()
}

// streamRound runs one model call, relaying content/reasoning deltas as SSE
// frames while accumulating tool-call fragments. ok=false means an error frame
// and done were already sent.
func (s *Server) streamRound(ctx context.Context, sink agentSink, provider ai.AIProvider, req ai.ChatRequest) (*ai.ChatResponse, bool) {
	stream, err := provider.ChatStream(ctx, req)
	if err != nil {
		s.log.Error("agent stream", "err", err)
		sink.fail("stream_error", "对话服务出了点问题，请稍后再试")
		return nil, false
	}
	var acc ai.StreamAccumulator
	for chunk := range stream {
		if chunk.Err != nil {
			s.log.Error("agent stream", "err", chunk.Err)
			sink.fail("stream_error", "对话流中断了，请重试")
			return nil, false
		}
		if chunk.ContentDelta != "" {
			sink.send(map[string]any{"type": "delta", "text": chunk.ContentDelta})
		}
		if chunk.ReasoningDelta != "" {
			sink.send(map[string]any{"type": "reasoning", "text": chunk.ReasoningDelta})
		}
		acc.Add(chunk)
	}
	return acc.Response(), true
}

// runProposeDecision emits a decision_card frame and blocks until the user
// answers, the wait times out, or a newer message cancels it. cancelled=true
// tells the loop to end this stream gracefully.
func (s *Server) runProposeDecision(ctx context.Context, sink agentSink, sid string, call ai.ToolCall) (res toolResult, cancelled bool) {
	var args struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Options []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"options"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil || len(args.Options) == 0 {
		return toolFail("propose_decision needs title and 2-4 options"), false
	}
	id := "dc_" + uuid.NewString()
	ch := s.decisions.register(sid, id)
	defer s.decisions.drop(id)

	sink.send(map[string]any{"type": "decision_card", "id": id, "title": args.Title, "summary": args.Summary, "options": args.Options})

	wait := decisionTimeoutSSE
	if dw, ok := sink.(decisionWaiter); ok {
		wait = dw.decisionWait()
	}
	timeout := time.NewTimer(wait)
	defer timeout.Stop()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case ans := <-ch:
			if ans.Choice == "cancelled" {
				return toolResult{OK: true, Data: map[string]any{"choice": "cancelled"}}, true
			}
			data := map[string]any{"choice": ans.Choice}
			if ans.Text != "" {
				data["text"] = ans.Text
			}
			return toolResult{OK: true, Data: data, Summary: args.Title}, false
		case <-timeout.C:
			return toolResult{OK: true, Data: map[string]any{"choice": "timeout"}, Summary: args.Title}, false
		case <-tick.C:
			sink.ping()
		case <-ctx.Done():
			return toolResult{OK: true, Data: map[string]any{"choice": "cancelled"}}, true
		}
	}
}

var wrapUpNudgeText = i18n.Text{
	"zh-CN": "（系统：工具轮次已用尽，请直接用文字总结目前的进展并回答用户，不要再调用任何工具。）",
	"en-US": "(System: tool rounds exhausted — summarize what happened and answer the user directly, without calling any more tools.)",
}

func wrapUpNudge(locale string) string { return i18n.Pick(wrapUpNudgeText, locale) }
