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
// all: the dev server (which proxies /api) and a deployment that serves the app
// from the same host as the API.
//
// # ⚠️ The keys are `daycore.*`, and the SHARING is the point
//
// They were `ting.*` until the fourth frontend, which is one frontend's name
// baked into the package the other three import — the exact thing this file's
// own comments object to elsewhere. But renaming is the small half. The real
// question a shared key raises is what happens when two of the four are served
// from ONE origin (example.com/ting/ and example.com/liuli-classic/): they see
// the same localStorage, so they share the address, the language, and the
// first-run flag.
//
// That is deliberate, and the argument is the scope itself: localStorage is
// keyed by ORIGIN, and an origin is exactly the granularity of "one
// deployment". Two deployments are two origins and two key spaces. So the
// sharing only ever happens between frontends that are, by construction,
// talking to the same install — where one address and one language is the right
// answer, and where making the reader configure the same backend twice would be
// the bug.
//
// The cost, stated: one origin serving two frontends against two DIFFERENT
// backends is not expressible. That configuration has no reason to exist, and
// dev servers are separate ports — separate origins — so it does not come up
// there either.

const BACKEND_KEY = 'daycore.backend';

/** The configured backend base URL, or "" meaning "same origin". */
export function backendBase(): string {
  try {
    return localStorage.getItem(BACKEND_KEY) ?? '';
  } catch {
    // Private mode, or storage disabled. Same-origin is the honest fallback:
    // it works where the app is co-served and fails visibly where it is not,
    // rather than half-working.
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

const SETUP_DONE_KEY = 'daycore.setupDone';

export function markSetupDone(): void {
  try {
    localStorage.setItem(SETUP_DONE_KEY, '1');
  } catch {
    /* the next launch asks again, which is the safe direction */
  }
}
