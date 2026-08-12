// One function per call. The comments here are the ones that cost somebody an
// afternoon to learn — a wrapper whose only job is remembering that `choice: ""`
// means REJECT earns its existence.

import { get, patch, post, rememberSession } from './http';
import type { DayPlan, Handshake, OperationLog, Proposal, Session } from './types';

export async function initSession(): Promise<Session> {
  // tokenInBody, because a separately-deployed frontend is cross-origin and
  // cannot rely on the cookie. See http.ts for the trade that carries.
  const s = await post<Session>('/api/session/init', { tokenInBody: true });
  if (s.sessionToken) rememberSession(s.sessionToken);
  return s;
}

export const handshake = (m: unknown) => post<Handshake>('/api/version', m);

/**
 * One day's plan. Null when the day is empty — a real answer, not an error.
 *
 * ⚠️ There is no /api/plan/today. An early draft of this client had one, copied
 * out of a grep that had picked the string up from a TEST FIXTURE
 * (internal/server/admin_gate_test.go). docs/API_SURFACE.md is generated from
 * the real route table and is the thing to check against.
 */
export const planForDate = (date: string) =>
  get<DayPlan | null>(`/api/plan?date=${encodeURIComponent(date)}`);

/** Today in the browser's own zone, as YYYY-MM-DD. */
export function todayIso(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/**
 * One incremental block edit.
 *
 * ⚠️ Can be REFUSED with 409 by the plan gate — the block is locked or has
 * petrified. That is why 汀 does not update its own state optimistically on
 * this call: a completion that silently bounced would leave the screen showing
 * a finished task the server still considers open, and 汀's whole premise is
 * that the screen answers "what now" truthfully.
 */
export const patchPlan = (date: string, action: unknown) =>
  patch<DayPlan>('/api/plan', { date, action });

export const proposals = () => get<{ proposals: Proposal[] }>('/api/proposals');

/**
 * Answer a proposal.
 *
 * ⚠️ The field is `choice`, and the server reads ANY value other than "reject"
 * or "" as an acceptance (internal/server/proposals.go). So a client that sends
 * `{}` meaning "accept" gets a REJECTION — silently, with a 200. That asymmetry
 * is deliberate on the server's side (silence must never accept anything), and
 * this wrapper exists so no caller here has to remember it.
 *
 * For a compound card, `choice` is the id of the row being taken and every other
 * row is rejected — so this signature does not cover those, and adding a row
 * picker later means a second function rather than an optional argument.
 */
export const respondToProposal = (id: string, accept: boolean) =>
  post<unknown>(`/api/proposals/${encodeURIComponent(id)}/respond`, {
    choice: accept ? 'accept' : 'reject',
  });

/** The field is `mood`, not `kind` — see internal/server/handlers_mood.go. */
export const recordMood = (mood: string, note = '') =>
  post<{ id: string }>('/api/mood', { mood, note });

export const ops = (limit = 5) => get<{ ops: OperationLog[] }>(`/api/ops?limit=${limit}`);

export const revertOp = (id: string) => post<unknown>(`/api/ops/${encodeURIComponent(id)}/revert`);

export const setTheme = (theme: string) => post<unknown>('/api/session/theme', { theme });

/** GET /api/version without introducing ourselves — used by the setting screen
 *  to check an address before committing to it. */
export const probe = () => get<{ version?: string; apiVersion?: number }>('/api/version');
