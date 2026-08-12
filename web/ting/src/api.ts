import { url } from './backend';

// The HTTP client. Shapes come from api/openapi.yaml, which is the only
// authoritative contract — design-ui/API_CONTRACT.md's path naming explicitly
// is not (design-ui/CLAUDE.md).

// ── session ────────────────────────────────────────────────────────────────
//
// ⚠️ 汀 uses the TOKEN, not the cookie, and that is a deployment decision rather
// than a preference. A separately-deployed frontend is cross-origin from the
// API, so the dc_sid cookie needs SameSite=None plus a correct ALLOWED_ORIGINS,
// and gets dropped entirely by browsers with third-party cookies off. The
// signed token in a header works the same everywhere and, per
// internal/server/server.go, a custom header forces a CORS preflight — which is
// what makes it CSRF-immune.
//
// The cost, stated: the token lives in localStorage, so an XSS in 汀 hands it
// over. A cookie would not have. That trade is accepted because 汀's whole
// premise is being deployed somewhere else, and a session that silently fails
// to persist is worse than one with a known exposure.

const TOKEN_KEY = 'ting.sessionToken';

function token(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

function setToken(t: string): void {
  try {
    localStorage.setItem(TOKEN_KEY, t);
  } catch {
    /* the session lasts this page load, which still works */
  }
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly body: unknown,
  ) {
    super(message);
  }
}

/** True when the failure is "this backend is not reachable / not a daycore". */
export function isUnreachable(e: unknown): boolean {
  return e instanceof ApiError && e.status === 0;
}

let buildHeader = '';

/** Set once, after the handshake — see themes and docs/specs/frontend-manifest.md. */
export function setBuildHeader(hash: string): void {
  buildHeader = hash;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  const t = token();
  if (t) headers.set('X-Session-Token', t);
  // ⚠️ Which token space the backend judges our themes against. NOT a
  // credential — it selects a vocabulary and nothing else.
  if (buildHeader) headers.set('X-Frontend-Build', buildHeader);

  let res: Response;
  try {
    res = await fetch(url(path), { ...init, headers });
  } catch (e) {
    // A network-level failure is the one an operator hits while typing a
    // backend address, so it gets a status of its own rather than being folded
    // into "something went wrong".
    throw new ApiError(0, 'unreachable', String(e), null);
  }

  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  if (!res.ok) {
    const b = body as { error?: string; message?: string } | null;
    throw new ApiError(res.status, b?.error ?? 'error', b?.message ?? res.statusText, body);
  }
  return body as T;
}

const get = <T,>(p: string) => request<T>(p);
const post = <T,>(p: string, body?: unknown) =>
  request<T>(p, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) });
const patch = <T,>(p: string, body: unknown) =>
  request<T>(p, { method: 'PATCH', body: JSON.stringify(body) });

// ── types ──────────────────────────────────────────────────────────────────

export interface TimeBlock {
  id: string;
  date?: string;
  time: string | null;
  title: string;
  type: 'task' | 'appointment' | 'break' | 'relax' | 'meal';
  duration_min: number | null;
  completed?: boolean;
  isAchievement?: boolean;
  origin?: 'auto' | 'manual' | 'rule';
  hidden?: boolean;
  lockLevel?: string;
  lockReason?: string;
  note?: string;
}

export interface DayPlan {
  date: string;
  blocks: TimeBlock[];
  note?: string;
}

export interface Proposal {
  id: string;
  state: 'pending' | 'accepted' | 'rejected' | 'expired';
  level: 'L1' | 'L2' | 'L3';
  kind: 'timed' | 'card' | 'decision';
  title: string;
  summary?: string;
  reason?: string;
  evidence?: string;
  date?: string;
  start?: string;
  dur?: number | null;
  btype?: string;
  expiresAt?: string;
}

export interface Session {
  id: string;
  assistantName: string;
  currentTheme: string;
  language?: string;
  sessionToken?: string;
}

export interface OperationLog {
  id: string;
  action: string;
  summary?: string;
  reverted?: boolean;
  revertedBy?: string;
  createdAt: string;
}

/** What POST /api/version answers. Fields the backend may omit are optional. */
export interface Handshake {
  apiVersion?: number;
  apiMinor?: number;
  version?: string;
  assignedFamilyId: string;
  handshakeRecorded: boolean;
  rulesAccepted?: boolean;
  newTokens?: string[];
  /** Kinds 汀 proposed that are waiting on a human — see manifest.ts. */
  pendingKinds?: string[];
  /** Tokens held back because their kind is still pending. */
  deferredTokens?: string[];
  pendingThemeBackfill?: number;
  note?: string;
  /** Every language this INSTALLATION can render — not a compile-time list.
   *  An operator dropping a file into LOCALES_DIR extends it. */
  locales?: { available: string[]; defaultPrimary: string; defaultSecondary?: string };
}

// ── calls ──────────────────────────────────────────────────────────────────

export async function initSession(): Promise<Session> {
  // tokenInBody, because 汀 is cross-origin and cannot rely on the cookie.
  const s = await post<Session>('/api/session/init', { tokenInBody: true });
  if (s.sessionToken) setToken(s.sessionToken);
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
