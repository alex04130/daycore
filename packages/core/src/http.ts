import { backendBase } from './backend';
import { apiPath } from './paths';

// The transport. Shapes come from api/openapi.yaml, which is the only
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
    res = await fetch(backendBase() + apiPath(path), { ...init, headers });
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

export const get = <T,>(p: string) => request<T>(p);
export const post = <T,>(p: string, body?: unknown) =>
  request<T>(p, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) });
export const patch = <T,>(p: string, body: unknown) =>
  request<T>(p, { method: 'PATCH', body: JSON.stringify(body) });

/** Stash the session token — boot() calls this after /api/session/init. */
export function rememberSession(t: string): void {
  setToken(t);
}
