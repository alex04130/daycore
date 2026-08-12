// Which backend this install talks to, and this build's identity.
//
// # ⚠️ The address is RUNTIME configuration, never a build-time constant
//
// docs/specs/frontend-manifest.md requires every frontend to ship a /setting
// first-install screen that can at least configure which backend to talk to,
// and the reason is stated there: "第三方前端写死后端地址就只能对着一个部署用，
// 而自部署是常态". A build baked against one URL is a build exactly one
// deployment can use — which for a project whose normal case is self-hosting
// means the artifact is useless to almost everybody who wants it.
//
// Default is the current origin, so the two easy cases need no configuration at
// all: the dev server (which proxies /api) and a deployment that serves 汀 from
// the same host as the API.

const BACKEND_KEY = 'ting.backend';

/** The configured backend base URL, or "" meaning "same origin". */
export function backendBase(): string {
  try {
    return localStorage.getItem(BACKEND_KEY) ?? '';
  } catch {
    // Private mode, or storage disabled. Same-origin is the honest fallback:
    // it works where 汀 is co-served and fails visibly where it is not, rather
    // than half-working.
    return '';
  }
}

export function setBackendBase(url: string): void {
  const clean = url.trim().replace(/\/+$/, '');
  try {
    if (clean === '') localStorage.removeItem(BACKEND_KEY);
    else localStorage.setItem(BACKEND_KEY, clean);
  } catch {
    /* nothing to do — the setting screen reports it */
  }
}

/** True when nobody has ever configured this install. Drives first-run /setting. */
export function isFirstRun(): boolean {
  try {
    return localStorage.getItem(SETUP_DONE_KEY) === null;
  } catch {
    return false;
  }
}

const SETUP_DONE_KEY = 'ting.setupDone';

export function markSetupDone(): void {
  try {
    localStorage.setItem(SETUP_DONE_KEY, '1');
  } catch {
    /* the next launch asks again, which is the safe direction */
  }
}

// ⚠️ The API lives under /api/v2 — see internal/apipath for why the version is
// in the path, and which routes stay outside it.
//
// Rewritten here rather than at every call site, for the same reason the server
// applies it in one mux wrapper: src/api.ts names RESOURCES, and which major
// serves them is one decision.
//
// ⚠️ Hardcoded for now, and that is a KNOWN LIMIT rather than a design. A build
// that is not version-locked ought to read the prefix from the handshake and
// refuse a backend whose major it does not speak — 汀 checks the major
// (session.ts) but still builds paths from a constant, so a v3 backend would
// give it 404s rather than a clear refusal. Fixing that means the handshake
// reporting its own prefix, which is a backend change.
const API_PREFIX = '/api/v2';
const UNVERSIONED = ['/api/version', '/api/healthz'];

export function apiPath(path: string): string {
  const bare = path.split('?')[0]!;
  if (!path.startsWith('/api/') || UNVERSIONED.includes(bare)) return path;
  if (path.startsWith(API_PREFIX + '/')) return path;
  return API_PREFIX + path.slice('/api'.length);
}

export function url(path: string): string {
  return backendBase() + apiPath(path);
}

// ── build identity ─────────────────────────────────────────────────────────

/**
 * This build's fingerprint.
 *
 * ⚠️ NOT a hash for security — a plain FNV-1a, and deliberately so. buildHash
 * identifies a build so the console can say "汀 has three builds connected"; the
 * backend states outright that the header carrying it is not a credential
 * (internal/server/themes.go). Reaching for SubtleCrypto here would buy nothing
 * and cost the case that matters most: it requires a secure context, so a
 * self-hosted deployment on plain http over a LAN — the normal case for this
 * project — would have no hash at all.
 *
 * It is derived rather than declared for the same reason the spec gives: a
 * declared build id can lie about being a different build. This one changes
 * exactly when the manifest or the version changes, which is when it should.
 */
export function buildHash(manifestJSON: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < manifestJSON.length; i++) {
    h ^= manifestJSON.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return 'ting-' + h.toString(16).padStart(8, '0');
}
