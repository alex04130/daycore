import * as api from './endpoints';
import { setBuildHeader } from './http';
import { markSetupDone } from './backend';
import { buildHash } from './build';
import { loadCatalog, preferredLocale, type Catalog, type Locale } from './i18n';
import type { Handshake, Session } from './types';

// Bringing a frontend up against a backend it has never met.
//
// Three steps, in this order, and the order is load-bearing:
//
//   1. handshake     introduce ourselves; learn which family we were assigned
//                    and which of our tokens this deployment can validate
//   2. build header  every later request carries it, so theme reads and writes
//                    are judged against OUR token space, not the fallback
//   3. session       the anonymous session everything else hangs off
//
// The handshake goes FIRST because it is the only step that works without a
// session — it has to, since a build making first contact holds no credential.
// Doing it after would mean a frontend that could not introduce itself until it
// had already started using the API it was introducing itself about.

export interface Boot {
  session: Session;
  handshake: Handshake;
  /** The reader's catalogue, built from what THIS deployment can render. */
  catalog: Catalog;
  /** Every locale the deployment offers — a setting screen lists them. */
  availableLocales: Locale[];
  /** Tokens this deployment will not accept yet, because their kind is pending. */
  deferred: string[];
  buildHash: string;
}

export interface BootProblem {
  kind: 'unreachable' | 'too-old' | 'error';
  message: string;
}

/** The oldest backend major any of these frontends knows how to talk to. */
export const MIN_API = 1;

/**
 * @param manifest  a function taking this build's hash and returning the body
 *                  POST /api/version wants. A FUNCTION rather than a value
 *                  because the hash is derived from the manifest itself, and
 *                  the caller is the only one that can close that loop.
 */
export async function boot(manifest: (hash: string) => unknown): Promise<Boot> {
  const hash = buildHash(JSON.stringify(manifest('')));
  setBuildHeader(hash);

  const hs = await api.handshake(manifest(hash));

  // ⚠️ A backend older than this build is REPORTED, not worked around. A
  // frontend could guess which calls still exist, and the guess would be wrong
  // in a way the user experiences as random breakage rather than as "these two
  // do not fit".
  if (hs.apiVersion !== undefined && hs.apiVersion < MIN_API) {
    throw { kind: 'too-old', message: String(hs.apiVersion) } satisfies BootProblem;
  }

  const session = await api.initSession();
  markSetupDone();

  // ⚠️ The language list comes from the HANDSHAKE, which is why the catalogue
  // is built here and not at module load. A frontend that picked its language
  // before asking the deployment what it can render would be choosing from a
  // list it made up — rule ② of docs/specs/frontend-manifest.md, and the one
  // place it is easy to get wrong.
  const available = hs.locales?.available ?? ['zh-CN'];
  const fallback = hs.locales?.defaultPrimary ?? available[0] ?? 'zh-CN';
  const catalog = await loadCatalog(preferredLocale(available, fallback), available, fallback);
  if (typeof document !== 'undefined') document.documentElement.lang = catalog.locale;

  return {
    session,
    handshake: hs,
    catalog,
    availableLocales: available,
    deferred: hs.deferredTokens ?? [],
    buildHash: hash,
  };
}
