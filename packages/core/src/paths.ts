// Where the API lives: /api/v2/…
//
// ⚠️ A MIRROR of internal/apipath, which is the authority. Two implementations
// of one rule, in two languages, and nothing in the build makes them meet —
// so when the backend's major moves, this file has to move with it, and the
// symptom of forgetting is every request 404ing at once.
//
// It is duplicated rather than fetched because a client has to know where to
// send its FIRST request, including the one that would have told it. See the
// note in boot.ts about what a version-negotiating client should do instead.

const API_PREFIX = '/api/v2';
const UNVERSIONED = ['/api/version', '/api/healthz'];

export function apiPath(path: string): string {
  const bare = path.split('?')[0]!;
  if (!path.startsWith('/api/') || UNVERSIONED.includes(bare)) return path;
  if (path.startsWith(API_PREFIX + '/')) return path;
  return API_PREFIX + path.slice('/api'.length);
}
