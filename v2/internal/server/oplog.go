package server

import (
	"context"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

// logOp appends one operation-log row and returns its id (the undo hook the
// agent's tool_result frames carry). Best-effort: the audit trail must never
// fail the operation it records.
func (s *Server) logOp(ctx context.Context, l *domain.OperationLog) string {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	if l.Actor == "" {
		l.Actor = domain.ActorUser
	}
	if l.Status == "" {
		l.Status = domain.OpStatusOK
	}
	if l.RequestID == "" {
		l.RequestID = requestIDFrom(ctx)
	}
	_ = s.store.OpLogs().Add(ctx, l)
	return l.ID
}
