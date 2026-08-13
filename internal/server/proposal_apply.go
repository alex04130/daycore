package server

import (
	"context"
	"encoding/json"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// Accepting a proposal runs what it said it would run.
//
// # ⚠️ Until this file existed, it did not
//
// handleProposalRespond flipped the state, wrote a ledger entry saying the user
// had accepted, and stopped. `Proposal.Ops` had zero writers AND zero
// executors, so a Protector card reading 「上午的安排我先帮你顺延」 was a button
// that did nothing — the card was honest about its intent and the code never
// carried it out. `protector.go` even says in a comment that accepting
// "performs the moves through the ordinary plan path"; that path had no caller.
//
// Nobody had noticed because the shipping web frontend does not call the
// proposals API at all, so the button did not exist yet on any screen. The four
// new frontends do. Building producers before this would have been building
// buttons that lie.
//
// # ⚠️ Why it goes through the agent tool path rather than writing directly
//
// domain.ProposalOp says it, and the reason is worth repeating where the code
// is: the tools already write the operation log and already have registered
// inverses. A second executor writing plans directly would be a second place
// where "every write is undoable" has to be remembered — and the first thing to
// be forgotten in it would be the undo.
//
// So an op is a tool call with the actor swapped: the same runCompanionTool the
// agent loop uses, invoked because a person pressed a button.

// applyProposalOps runs the ops an acceptance implies and returns the operation
// ids they produced, in order.
//
// ⚠️ Best-effort per op, and it does NOT stop at the first failure.
//
// The alternative — abort the rest — sounds safer and is worse here: the ops of
// one card are independent moves (postpone this, shorten that), not a
// transaction, and stopping halfway leaves the reader with a card marked
// accepted, some of its effects applied, and no way to ask for the remainder.
// Running them all means the failure is one missing move rather than an
// arbitrary prefix, and every move that DID happen is individually undoable
// because each wrote its own ledger entry.
//
// ⚠️ There is no compensation on partial failure, deliberately. Undoing the
// successes would need a second inverse path — the thing this file exists to
// avoid having — and it would also erase moves the reader may already have seen
// take effect.
func (s *Server) applyProposalOps(ctx context.Context, sid, locale, tz string, ops []domain.ProposalOp) []string {
	var opIDs []string
	for _, op := range ops {
		if op.Tool == "" {
			continue
		}
		args := "{}"
		if len(op.Args) > 0 {
			if b, err := json.Marshal(op.Args); err == nil {
				args = string(b)
			}
		}
		// ⚠️ propose_decision is refused here rather than handled. It is the one
		// tool that blocks waiting for a human, and reaching it from an
		// acceptance would mean a card whose acceptance opens another card —
		// with nothing on the other end to answer it, because this path has no
		// sink. runCompanionTool would not route it anyway (the agent loop
		// special-cases it before calling), so this is a guard against a future
		// caller rather than against today's.
		if op.Tool == "propose_decision" {
			s.log.Error("proposal op asked for a decision card; ignoring", "session", sid)
			continue
		}
		res := s.runCompanionTool(ctx, sid, locale, tz, ai.ToolCall{Name: op.Tool, Arguments: args})
		if !res.OK {
			// Logged rather than returned: the caller has already told the user
			// their answer was recorded, and the answer WAS recorded. What
			// failed is one of its consequences.
			s.log.Error("proposal op failed", "session", sid, "tool", op.Tool, "err", res.ErrMsg)
			continue
		}
		if res.OpID != "" {
			opIDs = append(opIDs, res.OpID)
		}
	}
	return opIDs
}

// opsToApply picks the ops an answer implies.
//
// ⚠️ A compound card runs the ops of the ACCEPTED ROW, not the card's own. A
// card with rows is a menu — running the card-level ops as well would apply the
// thing the reader chose plus the thing they chose it instead of.
//
// A simple card (no rows) runs its own. Both shapes exist: the Protector's card
// is simple, a conflict card is a menu.
func opsToApply(p *domain.Proposal, choice string) []domain.ProposalOp {
	if len(p.Rows) == 0 {
		return p.Ops
	}
	for _, row := range p.Rows {
		if row.ID == choice {
			return row.Ops
		}
	}
	return nil
}
