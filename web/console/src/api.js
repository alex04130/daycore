// The console's only way to talk to the backend.
//
// # Authentication: the cookie, never the token
//
// There are two admin credentials and this file uses exactly one of them.
//
//   X-Admin-Token   a long-lived environment variable — for curl and CI
//   dc_admin        an httpOnly, Secure, SameSite=Strict cookie with a short TTL,
//                   minted by POST /api/admin/session in exchange for the token
//
// The design prototype kept the raw token in sessionStorage and re-sent it as a
// header on every request (design-ui/liuli/admin/admin-store.js:19). That is
// precisely what θ-F4a existed to remove: a long-lived credential, in reach of
// any script on the page, with no expiry, no revocation and no identity. This
// file never stores the token — it posts it once, receives a cookie it cannot
// read, and from then on sends nothing at all.
//
// ⚠️ Which is why every request here is `credentials: 'same-origin'` and why
// nothing in this module accepts an absolute URL. A console that could be
// pointed at another origin would send its cookie there.

const BASE = '/api/admin';

// Sentinel for "the credential is gone". Thrown rather than returned so no
// caller can forget to check it and render an empty screen where a login
// prompt belongs.
export class Unauthorized extends Error {
  constructor() {
    super('unauthorized');
    this.name = 'Unauthorized';
  }
}

// Degraded is storage being unavailable, which is a DIFFERENT thing from
// broken. The console is specifically expected to work in this state — that is
// what degraded boot is for — so screens that read no rows must keep rendering
// and screens that do must say why they cannot.
export class Degraded extends Error {
  constructor(message) {
    super(message || 'storage unavailable');
    this.name = 'Degraded';
  }
}

async function request(path, { method = 'GET', body } = {}) {
  const res = await fetch(BASE + path, {
    method,
    // same-origin, always: the admin cookie must never travel to another host.
    credentials: 'same-origin',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401) throw new Unauthorized();

  let data = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      // A non-JSON body from an endpoint that promises JSON means something
      // upstream answered instead — a proxy, an error page. Say that, rather
      // than a parse error nobody can act on.
      throw new Error(`${method} ${path}: the server did not return JSON (HTTP ${res.status})`);
    }
  }

  if (res.status === 503) throw new Degraded(data?.message);
  if (!res.ok) {
    // The server's own message, which is localised and written for a person.
    // Falling back to the code is better than "Error 400".
    throw new Error(data?.message || data?.error || `${method} ${path}: HTTP ${res.status}`);
  }
  return data;
}

// ── session ────────────────────────────────────────────────────────────────

// login exchanges the admin token for a short-lived httpOnly cookie.
//
// The token is passed straight through and never kept: it exists in a form
// field for as long as the request takes and then it is gone. Anything else —
// a variable, storage, a retry buffer — is a long-lived credential in the page.
export const login = (token) => request('/session', { method: 'POST', body: { token } });
export const logout = () => request('/session', { method: 'DELETE' });

// ── the eight sections ─────────────────────────────────────────────────────

export const getConfig = () => request('/config');
export const putConfig = (settings) => request('/config', { method: 'PUT', body: { settings } });

export const getProviders = () => request('/providers');
export const putProviders = (providers) => request('/providers', { method: 'PUT', body: { providers } });

export const getModels = () => request('/models');
export const testModel = (id) => request(`/models/${encodeURIComponent(id)}/test`, { method: 'POST' });

export const getOAuth = () => request('/oauth');

// ── meta (not under /api/admin) ────────────────────────────────────────────

// Health and version are public endpoints, so they are fetched without the
// admin prefix. They are what the overview shows BEFORE a credential exists,
// which is the state an operator is in when they are trying to find out why
// nothing works.
export async function getMeta() {
  const [health, version] = await Promise.all([
    fetch('/api/healthz', { credentials: 'same-origin' })
      .then((r) => r.json().then((j) => ({ ...j, status: r.status })))
      // A health endpoint that cannot be reached is itself the answer, and it
      // must not take the whole screen down with it.
      .catch(() => ({ ok: false, status: 0, error: 'unreachable' })),
    fetch('/api/version', { credentials: 'same-origin' })
      .then((r) => r.json())
      .catch(() => null),
  ]);
  return { health, version };
}
