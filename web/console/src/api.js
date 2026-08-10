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

// loginAsMyself exchanges the signed-in user's OWN login for a console session.
//
// An empty body is the whole request: the credential is the dc_auth cookie the
// browser already holds, and this endpoint is what turns it into a console
// cookie whose permissions come from that person's groups. Somebody with no
// console permission is refused here rather than handed a credential that can
// do nothing.
export const loginAsMyself = () => request('/session', { method: 'POST', body: {} });

export const logout = () => request('/session', { method: 'DELETE' });

// whoami is the first call after a reload: who is holding this session and what
// may they do.
//
// ⚠️ Its answer drives which sections render, and that is a COURTESY. Every
// endpoint checks for itself — hiding a screen is so nobody clicks into a 403,
// not a substitute for the server refusing.
export const whoami = () => request('/session');

// ── the eight sections ─────────────────────────────────────────────────────

// restart really restarts the process — it drains, spawns a replacement, and
// only then lets go. Expect a few seconds of refused connections after a 200.
export const restart = () => request('/restart', { method: 'POST' });

export const getConfig = () => request('/config');
export const putConfig = (settings) => request('/config', { method: 'PUT', body: { settings } });

export const getProviders = () => request('/providers');
export const putProviders = (providers) => request('/providers', { method: 'PUT', body: { providers } });

export const getModels = () => request('/models');
export const testModel = (id) => request(`/models/${encodeURIComponent(id)}/test`, { method: 'POST' });

export const getOAuth = () => request('/oauth');

export const getStats = () => request('/stats');

// getUsage is the spend rollup: a per-day series and a per-model fold.
//
// ⚠️ It stops at `throughDay` — today is never in it, because a day is folded
// only once it can no longer receive rows. /stats is the one place that adds
// today's live rows on top.
export function getUsage(params = {}) {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v !== '' && v !== null && v !== undefined) q.set(k, v);
	}
	const s = q.toString();
	return request('/usage' + (s ? '?' + s : ''));
}

// getAILogs takes the filter as an object and drops the empty fields, because
// `?status=` is not the same request as omitting it — the server refuses an
// unknown status, and an empty string is one.
export function getAILogs(params = {}) {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v !== '' && v !== null && v !== undefined) q.set(k, v);
	}
	const s = q.toString();
	return request('/ailogs' + (s ? '?' + s : ''));
}

export const getPrompts = () => request('/prompts');
export const getPrompt = (key, locale) =>
	request(`/prompts/${encodeURIComponent(key)}?locale=${encodeURIComponent(locale)}`);
export const putPrompt = (key, locale, content) =>
	request(`/prompts/${encodeURIComponent(key)}?locale=${encodeURIComponent(locale)}`, {
		method: 'PUT',
		body: { content },
	});

export const getDBTables = () => request('/db/tables');
export const getDBTable = (name, limit, offset) =>
	request(`/db/table/${encodeURIComponent(name)}?limit=${limit}&offset=${offset}`);
export const deleteDBRow = (name, id) =>
	request(`/db/table/${encodeURIComponent(name)}/${encodeURIComponent(id)}`, { method: 'DELETE' });

// exportDB is a link, not a fetch: the response is a file with a
// Content-Disposition, and routing it through fetch would mean holding the whole
// database in memory to hand it back to a download the browser does natively.
export const exportURL = () => BASE + '/db/export';

export const getPairings = () => request('/pairings');

// createPairing returns the key ONCE. Nothing stores it, nothing can fetch it
// again — see the notice the server sends back with it.
export const createPairing = (body) => request('/pairings', { method: 'POST', body });
export const setPairingRoles = (id, roles) =>
	request(`/pairings/${encodeURIComponent(id)}/roles`, { method: 'PUT', body: { roles } });
// ⚠️ Root credential only, server-side. Offered to everyone and the 403
// explains — the same rule the owner mark follows, so nobody is left wondering
// where the control went.
export const setPairingFull = (id, full) =>
	request(`/pairings/${encodeURIComponent(id)}/full`, { method: 'PUT', body: { full } });

export const deletePairing = (id) =>
	request(`/pairings/${encodeURIComponent(id)}`, { method: 'DELETE' });

export const getUsers = () => request('/users');
export const getRoles = () => request('/roles');
export const getPermissions = () => request('/permissions');

// putRole REPLACES what a group grants — the body is the new set, because a
// merge cannot express "take this away".
export const putRole = (name, body) =>
  request(`/roles/${encodeURIComponent(name)}`, { method: 'PUT', body });
export const deleteRole = (name) =>
  request(`/roles/${encodeURIComponent(name)}`, { method: 'DELETE' });

// putUserRoles sends the whole membership set for one person, for the same
// reason: an add/remove pair is two requests that can half-apply.
export const putUserRoles = (id, roles) =>
  request(`/users/${encodeURIComponent(id)}/roles`, { method: 'PUT', body: { roles } });

// putUserOwner is root-credential-only on the server. The console shows the
// control to everybody and lets the 403 explain, rather than hiding the one
// path out of a locked-out deployment.
export const putUserOwner = (id, owner) =>
  request(`/users/${encodeURIComponent(id)}/owner`, { method: 'PUT', body: { owner } });

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
