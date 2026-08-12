// 汀's message catalogue.
//
// docs/specs/frontend-manifest.md, "前端的多语言形态", states four requirements
// and names the thing not to copy — web/frontend/src/i18n.js, a hardcoded
// bilingual dictionary inside a .js file. Each rule below says which one it is.
//
// The whole point, from the backend's side: "加一门语言 = 丢一个 JSON 文件".
// That sentence is only true of the product if it is true of the frontend too.

export type Locale = string;
export type Pack = Record<string, string>;

/** ① Copy is DATA, not an object literal in code.
 *
 * Packs live in public/locales/<locale>.json and are FETCHED at runtime, so a
 * built artifact can gain a language by dropping a file next to it — no
 * rebuild, no release. Importing them would inline them into the bundle and
 * quietly turn this back into a compile-time dictionary, which is the failure
 * this rule exists to prevent. */
const packURL = (locale: Locale) => `locales/${locale}.json`;

const loaded = new Map<Locale, Pack>();

async function loadPack(locale: Locale): Promise<Pack | null> {
  if (loaded.has(locale)) return loaded.get(locale)!;
  try {
    // Relative to the document, so it follows vite's base:'./' wherever 汀 is
    // deployed — a root, a subpath, a CDN, a file:// bundle.
    const res = await fetch(new URL(packURL(locale), document.baseURI).toString());
    if (!res.ok) return null;
    const pack = (await res.json()) as Pack;
    loaded.set(locale, pack);
    return pack;
  } catch {
    return null;
  }
}

/** The base language of a tag: zh-TW → zh. */
function base(locale: Locale): string {
  return (locale.split('-')[0] ?? locale).toLowerCase();
}

/** ③ The fallback chain, matching the backend's:
 *
 *     exact → same base language → default → any
 *
 * ⚠️ The last step is deliberate and is the one that looks wrong. Showing a
 * half-translated interface in a language the reader may not know beats showing
 * blanks, because a missing translation is a CONTENT gap and a blank screen
 * reads as a RENDERING FAULT. People work around the first and file bugs about
 * the second. */
export function chainFor(want: Locale, available: Locale[], fallback: Locale): Locale[] {
  const chain: Locale[] = [];
  const add = (l: Locale | undefined) => {
    if (l && !chain.includes(l)) chain.push(l);
  };
  add(available.find((l) => l.toLowerCase() === want.toLowerCase()));
  add(available.find((l) => base(l) === base(want)));
  add(available.find((l) => l.toLowerCase() === fallback.toLowerCase()));
  for (const l of available) add(l);
  return chain;
}

export interface Catalog {
  locale: Locale;
  /** Every locale actually loaded, in fallback order. */
  chain: Locale[];
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const PLACEHOLDER = /\{(\w+)\}/g;

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(PLACEHOLDER, (whole, name: string) =>
    // ⚠️ An unknown placeholder is left as-is rather than emptied. `{n} 分钟`
    // rendering as ` 分钟` looks like a layout bug; rendering as `{n} 分钟`
    // looks like what it is — a translation that names a variable this call
    // does not pass.
    Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : whole,
  );
}

/**
 * Build a catalog for one reader.
 *
 * ② `available` comes from the BACKEND (GET /api/version → locales.available),
 * never from a constant here. An operator who drops a file into LOCALES_DIR adds
 * a language to that deployment; a frontend that hardcoded the list it thinks
 * exists would simply never offer it, and nothing anywhere would report why.
 */
export async function loadCatalog(
  want: Locale,
  available: Locale[],
  fallback: Locale,
): Promise<Catalog> {
  const chain = chainFor(want, available, fallback);
  const packs: Pack[] = [];
  const got: Locale[] = [];
  for (const locale of chain) {
    const p = await loadPack(locale);
    if (p) {
      packs.push(p);
      got.push(locale);
    }
  }

  const t = (key: string, vars?: Record<string, string | number>): string => {
    for (const p of packs) {
      const v = p[key];
      if (v !== undefined && v !== '') return interpolate(v, vars);
    }
    // ④ A missing key shows the KEY. Not an empty string (reads as a layout
    // bug), not an invented string (reads as a mistranslation). A key on screen
    // is unmistakably "nobody has written this yet" and it says which one.
    return key;
  };

  return { locale: got[0] ?? want, chain: got, t };
}

/** What the browser says the reader prefers, most-wanted first. */
export function browserLocales(): Locale[] {
  const nav = typeof navigator === 'undefined' ? undefined : navigator;
  const list = nav?.languages?.length ? [...nav.languages] : nav?.language ? [nav.language] : [];
  return list.filter(Boolean);
}

/** The reader's own choice, if they made one on the setting screen. */
const CHOICE_KEY = 'ting.locale';

// ⚠️ An in-memory copy, because localStorage is not always there — private
// browsing, storage disabled, an embedded webview.
//
// The first version only wrote to localStorage and swallowed the failure with
// "the choice lasts this page load". It did not: preferredLocale re-reads
// storage on every call, so in private mode picking a language did nothing AT
// ALL, silently, and the setting screen would have shown the choice not taking
// effect with no explanation. Persisting is the part that can degrade; taking
// effect is not.
let inMemoryChoice: Locale | null = null;

export function chosenLocale(): Locale | null {
  if (inMemoryChoice !== null) return inMemoryChoice;
  try {
    return localStorage.getItem(CHOICE_KEY);
  } catch {
    return null;
  }
}

export function chooseLocale(locale: Locale | null): void {
  inMemoryChoice = locale;
  try {
    if (locale === null) localStorage.removeItem(CHOICE_KEY);
    else localStorage.setItem(CHOICE_KEY, locale);
  } catch {
    /* it will not survive a reload; it does take effect now */
  }
}

/**
 * Which locale to render in: the reader's explicit choice, else what their
 * browser asks for, else the deployment's default.
 *
 * ⚠️ The browser's preference is consulted against what the DEPLOYMENT has, not
 * against a list 汀 believes in — that is rule ② again, at the one place it is
 * easy to get wrong.
 */
export function preferredLocale(available: Locale[], fallback: Locale): Locale {
  const chosen = chosenLocale();
  if (chosen) return chosen;
  for (const want of browserLocales()) {
    const hit =
      available.find((l) => l.toLowerCase() === want.toLowerCase()) ??
      available.find((l) => base(l) === base(want));
    if (hit) return hit;
  }
  return fallback;
}

/**
 * A catalogue for the screens that run BEFORE any backend has been reached:
 * the first-run setting screen, and the "could not connect" screen.
 *
 * ⚠️ This does not break rule ②. That rule forbids hardcoding what the
 * DEPLOYMENT offers — and before 汀 has talked to a deployment there is no
 * deployment list to read. What it can honestly use is the set of packs it
 * SHIPS WITH, discovered the same way as any other: by asking for them and
 * seeing which arrive. An operator's extra language appears the moment the real
 * catalogue replaces this one, which is as soon as the handshake answers.
 *
 * SHIPPED is the list of packs in public/locales/. It is the one place a
 * compile-time language list is correct, because it describes this artifact
 * rather than that installation.
 */
const SHIPPED: Locale[] = ['zh-CN', 'en-US'];

export function bootstrapCatalog(): Promise<Catalog> {
  return loadCatalog(preferredLocale(SHIPPED, SHIPPED[0]!), SHIPPED, SHIPPED[0]!);
}
