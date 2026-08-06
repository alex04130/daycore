package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("temp-context (session-scoped key-value store)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/temp-context", s.handleTempContextGet)
		mux.HandleFunc("PUT /api/temp-context", s.handleTempContextPut)
	})
}

// maxPayloadLen limits the size of a single temp-context payload.
const maxPayloadLen = 64 << 10 // 64 KiB

// StartTempContextCleanup spawns a background goroutine that periodically expires
// stale temp-context entries. Call this once at server startup.
func (s *Server) StartTempContextCleanup(interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	s.everyTick("temp-context expire", interval, func(ctx context.Context) {
		n, err := s.store.TempContexts().Expire(ctx)
		if err != nil {
			s.log.Warn("temp-context expire error", slog.String("err", err.Error()))
			return
		}
		if n > 0 {
			s.log.Info("temp-context expired stale entries", slog.Int("count", n))
		}
	})
}

// GET /api/temp-context?key=...
func (s *Server) handleTempContextGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.tempContextGet.bad_request")
		return
	}
	tc, err := s.store.TempContexts().Get(r.Context(), sid, key)
	if errors.Is(err, domain.ErrNotFound) {
		// Missing/expired/rotated key → data:null, not a 500.
		s.writeJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.tempContextGet.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": tc.Payload, "ttl": tc.TTL.UTC().Format(time.RFC3339)})
}

// PUT /api/temp-context
func (s *Server) handleTempContextPut(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Key        string `json:"key"`
		Payload    string `json:"payload"`
		TTLSeconds int    `json:"ttlSeconds"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.tempContextPut.bad_request")
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	if body.Key == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.tempContextPut.bad_request2")
		return
	}
	if len(body.Payload) > maxPayloadLen {
		s.writeErr(w, http.StatusBadRequest, "bad_request",
			fmt.Sprintf("payload 过长，最大 %d 字节", maxPayloadLen))
		return
	}
	ttl := time.Duration(body.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour // default
	}
	if ttl > 7*24*time.Hour {
		ttl = 7 * 24 * time.Hour // cap at 7 days
	}

	tc := &domain.TempContext{
		SessionID: sid,
		Key:       body.Key,
		Payload:   body.Payload,
		TTL:       time.Now().Add(ttl),
	}
	if err := s.store.TempContexts().Set(r.Context(), tc); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.tempContextPut.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ttl": tc.TTL.UTC().Format(time.RFC3339)})
}
