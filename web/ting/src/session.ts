import * as api from './api';
import { buildHash, markSetupDone } from './backend';
import { manifest, TOKENS } from './manifest';
import { loadCatalog, preferredLocale, type Catalog } from './i18n';

// Bringing 汀 up against a backend it has never met.
//
// Three steps, in this order, and the order is load-bearing:
//
//   1. handshake   introduce ourselves, learn which family we were assigned and
//                  which of our tokens the deployment can actually validate
//   2. build header  every later request carries it, so theme reads and writes
//                  are judged against OUR token space rather than the fallback
//   3. session     the anonymous session everything else hangs off
//
// The handshake goes FIRST because it is the only step that works without a
// session — it has to, since a frontend making first contact holds no
// credential. Doing it after would mean a build that could not introduce itself
// until it had already started using the API it was introducing itself about.

export interface Boot {
  session: api.Session;
  handshake: api.Handshake;
  /** The reader's catalogue, built from what THIS deployment can render. */
  catalog: Catalog;
  /** Every locale this deployment offers — the setting screen lists them. */
  availableLocales: string[];
  /** Tokens this deployment will not accept yet, because their kind is pending. */
  deferred: string[];
  buildHash: string;
}

export interface BootProblem {
  kind: 'unreachable' | 'too-old' | 'error';
  message: string;
}

const MANIFEST_JSON = JSON.stringify(manifest('')); // hash input: everything but the hash

export const BUILD_HASH = buildHash(MANIFEST_JSON);

export async function boot(): Promise<Boot> {
  api.setBuildHeader(BUILD_HASH);

  const hs = await api.handshake(manifest(BUILD_HASH));

  // ⚠️ A backend older than this build is reported, not worked around. 汀 could
  // guess at which calls still exist, and the guess would be wrong in a way the
  // user experiences as random breakage rather than as "these two do not fit".
  if (hs.apiVersion !== undefined && hs.apiVersion < 1) {
    throw { kind: 'too-old', message: String(hs.apiVersion) } satisfies BootProblem;
  }

  const session = await api.initSession();
  markSetupDone();

  // ⚠️ The language list comes from the HANDSHAKE, which is why the catalogue is
  // built here and not at module load. A frontend that picked its language
  // before asking the deployment what it can render would be choosing from a
  // list it made up — rule ② of docs/specs/frontend-manifest.md, and the one
  // place it is easy to get wrong.
  const available = hs.locales?.available ?? ['zh-CN'];
  const fallback = hs.locales?.defaultPrimary ?? available[0] ?? 'zh-CN';
  const catalog = await loadCatalog(preferredLocale(available, fallback), available, fallback);
  document.documentElement.lang = catalog.locale;

  return {
    session,
    handshake: hs,
    catalog,
    availableLocales: available,
    deferred: hs.deferredTokens ?? [],
    buildHash: BUILD_HASH,
  };
}

/**
 * Which tokens this deployment cannot theme yet.
 *
 * ⚠️ 汀 keeps working without them — its stylesheet's own values apply, and the
 * only thing missing is the ability to change that one value from a theme. That
 * degradation is the entire point of the propose-then-approve design: a
 * frontend that refused to start because an operator had not read a regex yet
 * would have made the third tier a blocker instead of a feature.
 */
export function deferredTokenNames(hs: api.Handshake): string[] {
  const deferred = new Set(hs.deferredTokens ?? []);
  return TOKENS.filter((t) => deferred.has(t.name)).map((t) => t.name);
}
