import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { PAGES } from './App';
import { MATERIAL_TABS } from './PageMaterials';
import { CARE_SWITCHES } from './PageSettings';
import { EXERCISES } from './PageMood';

// The gate that keeps the language packs honest.
//
// ⚠️ Without something like this, i18n rots in exactly one direction: somebody
// adds a string, writes the zh-CN value, and the en-US pack silently grows a
// hole that renders as a bare key on a reader's screen. The backend has the
// same gate (make check-i18n) for the same reason.
//
// It is a TEST rather than a script so it runs in `npm run build` — a check
// nobody remembers to run is a check that does not exist.
//
// # ⚠️ 初版 needed two things the other three did not, and both are the same
//   failure mode: a gate that is wrong in the direction of "delete this
//   translation".
//
// The first three frontends have one surface and no data module that produces
// copy, so every key they use appears as a literal inside a t(...) call. 初版
// has neither property:
//
//   - a page LIST, so the nav renders `t(`nav.${id}`)` over five ids;
//   - src/rules.ts, which returns catalogue KEYS as data (that is the whole
//     point of it — see the file), so `rules.weekly` never appears inside a
//     t(...) call anywhere.
//
// Under the older gate every one of those keys reads as an orphan, and the
// suggested fix is to delete a translation that is in use. That is strictly
// worse than having no check, so the extraction below grew two more rules —
// each mechanical, neither requiring anybody to maintain a list.

const LOCALES_DIR = join(import.meta.dirname, '..', 'public', 'locales');
const SRC_DIR = import.meta.dirname;

function packs(): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const f of readdirSync(LOCALES_DIR)) {
    if (f.endsWith('.json')) {
      out[f.replace(/\.json$/, '')] = JSON.parse(readFileSync(join(LOCALES_DIR, f), 'utf8'));
    }
  }
  return out;
}

function sourceFiles(): string[] {
  return readdirSync(SRC_DIR).filter(
    (f) => (f.endsWith('.ts') || f.endsWith('.tsx')) && !f.endsWith('.test.ts'),
  );
}

function sources(): string[] {
  return sourceFiles().map((f) => readFileSync(join(SRC_DIR, f), 'utf8'));
}

/** The text of every `t(...)` call in the app. */
function tCalls(): string[] {
  const calls: string[] = [];
  for (const src of sources()) {
    for (let i = src.indexOf('t('); i !== -1; i = src.indexOf('t(', i + 1)) {
      // Only a real call: `t(` preceded by a boundary, so `split(` and
      // `parseInt(` do not qualify.
      const before = i === 0 ? ' ' : src[i - 1]!;
      if (/[\w$.]/.test(before)) continue;
      let depth = 0;
      let end = i + 1;
      for (; end < src.length; end++) {
        if (src[end] === '(') depth++;
        else if (src[end] === ')') {
          depth--;
          if (depth === 0) break;
        }
      }
      calls.push(src.slice(i, end));
    }
  }
  return calls;
}

// ⚠️ Dotted only. Scanning the whole call also picks up the values a ternary
// COMPARES against — `t(p.level === 'L3' ? … )` yields 'L3' — and reporting
// those as undefined keys is a check that cries wolf. Every key is dotted, and
// a test below enforces that so this discriminator cannot silently stop working.
const DOTTED = /['"]([a-z][\w]*(?:\.[\w]+)+)['"]/g;

/**
 * Every key the source asks for BY NAME.
 *
 * ⚠️ Scans the whole `t(...)` call, not just its first argument. The obvious
 * regex — `t\(\s*'key'` — misses every key chosen by a ternary
 * (`t(x ? 'a.b' : 'c.d')`), which is the natural way to write two-state copy.
 * An early version of this gate reported those as orphans.
 */
function usedKeys(): Set<string> {
  const keys = new Set<string>();
  for (const call of tCalls()) {
    for (const m of call.matchAll(DOTTED)) keys.add(m[1]!);
  }
  return keys;
}

/**
 * Prefixes of keys built from a template literal: `` t(`nav.${id}`) `` → `nav.`
 *
 * ⚠️ Derived from the source rather than declared in a list here. A declared
 * list is a second place to remember, and the whole reason this file exists is
 * that remembering does not work.
 *
 * The cost, stated: everything under an exempt prefix is unverifiable, so a
 * typo'd `nav.todya` in a pack would not be reported. That is the acceptable
 * direction — the alternative reports keys that ARE in use and tells a reader
 * to delete them.
 */
function usedPrefixes(): string[] {
  const out = new Set<string>();
  for (const call of tCalls()) {
    for (const m of call.matchAll(/`([a-z][\w.]*\.)\$\{/g)) out.add(m[1]!);
  }
  return [...out];
}

/**
 * Dotted literals in a `key:` position anywhere in the app.
 *
 * ⚠️ For src/rules.ts, which returns `{ key: 'rules.weekly', vars }` as DATA.
 * Those keys reach `t` through a variable, so no amount of scanning t(...)
 * calls will ever see them — and a module that returns keys instead of
 * sentences is the pattern this project wants, not an exception to tolerate.
 */
function keyLiterals(): Set<string> {
  const out = new Set<string>();
  for (const src of sources()) {
    for (const m of src.matchAll(/\bkey:\s*['"]([a-z][\w]*(?:\.[\w]+)+)['"]/g)) out.add(m[1]!);
  }
  return out;
}

describe('language packs', () => {
  const all = packs();
  const names = Object.keys(all).sort();

  it('ships at least the two the build knows about', () => {
    expect(names).toContain('zh-CN');
    expect(names).toContain('en-US');
  });

  // ⚠️ The alignment check. A key present in one pack and missing from another
  // renders as a bare key for the readers of the second — visible, but only to
  // the people least likely to report it.
  it('has exactly the same keys in every pack', () => {
    const reference = new Set(Object.keys(all['zh-CN']!));
    for (const name of names) {
      const here = new Set(Object.keys(all[name]!));
      const missing = [...reference].filter((k) => !here.has(k));
      const extra = [...here].filter((k) => !reference.has(k));
      expect({ locale: name, missing, extra }).toEqual({ locale: name, missing: [], extra: [] });
    }
  });

  // ⚠️ Load-bearing for the extraction above, which tells a key from a
  // comparison value by the dot. A flat key would be invisible to the gate — so
  // the convention is asserted rather than assumed.
  it('names every key with a dot, which is what the gate keys off', () => {
    const flat = Object.keys(all['zh-CN']!).filter((k) => !k.includes('.'));
    expect(flat).toEqual([]);
  });

  it('has no empty values, which read as missing', () => {
    for (const name of names) {
      const blank = Object.entries(all[name]!)
        .filter(([, v]) => v.trim() === '')
        .map(([k]) => k);
      expect({ locale: name, blank }).toEqual({ locale: name, blank: [] });
    }
  });

  it('defines every key the source asks for by name', () => {
    const defined = new Set(Object.keys(all['zh-CN']!));
    const undefinedKeys = [...usedKeys(), ...keyLiterals()].filter((k) => !defined.has(k)).sort();
    expect(undefinedKeys).toEqual([]);
  });

  it('has nothing nobody asks for', () => {
    const used = usedKeys();
    const literals = keyLiterals();
    const prefixes = usedPrefixes();
    const orphans = Object.keys(all['zh-CN']!)
      .filter((k) => !used.has(k) && !literals.has(k) && !prefixes.some((p) => k.startsWith(p)))
      .sort();
    expect(orphans).toEqual([]);
  });

  // ⚠️ Every dynamic family must actually have members. Without this, deleting
  // the last `nav.*` key leaves the exemption in place and the nav renders five
  // bare keys with the suite still green — the exemption turning into the hole
  // it was meant to avoid.
  it('has at least one key under every dynamically-built prefix', () => {
    const keys = Object.keys(all['zh-CN']!);
    for (const p of usedPrefixes()) {
      expect({ prefix: p, has: keys.some((k) => k.startsWith(p)) }).toEqual({ prefix: p, has: true });
    }
  });

  // ⚠️ The prefix exemption above says "everything under an exempt prefix is
  // unverifiable", and that is true only for families whose members are not
  // known until runtime. Four of the five ARE known — they are arrays in the
  // source — so they get checked member by member rather than waved through.
  //
  // The one that genuinely cannot be: `theme.${id}` comes from the BACKEND's
  // built-in list, so a deployment that ships a fifth theme renders a bare key
  // here. That gap is closed on the Go side, where the list actually lives, by
  // internal/theme/frontend_manifest_test.go.
  //
  // Verified by deleting one member of each family and watching this fail —
  // which is exactly the case the exemption alone let through.
  it('defines every member of the families whose domain is in the source', () => {
    const defined = new Set(Object.keys(all['zh-CN']!));
    const expected = [
      ...PAGES.map((p) => `nav.${p}`),
      ...MATERIAL_TABS.map((x) => `materials.tab.${x}`),
      ...CARE_SWITCHES.map((x) => `settings.pref.${String(x)}`),
      ...EXERCISES.map((x) => `mood.exercise.${x}`),
    ];
    expect(expected.filter((k) => !defined.has(k))).toEqual([]);
  });

  // ⚠️ Placeholders must match across languages, or a translation renders
  // `{n} 分钟` as ` 分钟` in one language and correctly in another — and the
  // broken one is whichever the reviewer does not read.
  it('uses the same placeholders in every language', () => {
    const ph = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]!).sort();
    for (const key of Object.keys(all['zh-CN']!)) {
      const reference = ph(all['zh-CN']![key]!);
      for (const name of names) {
        expect({ key, locale: name, ph: ph(all[name]![key]!) }).toEqual({
          key,
          locale: name,
          ph: reference,
        });
      }
    }
  });
});

describe('no hardcoded copy left in the app', () => {
  // ⚠️ CJK in a string literal in application source is copy that escaped the
  // catalogue. Comments are exempt — they are for the people reading the code,
  // not the people using it.
  it('has no CJK string literals outside comments', () => {
    const offenders: string[] = [];
    for (const f of sourceFiles()) {
      // manifest.ts is the exception, and a real one: its strings are 初版's own
      // identity and the token descriptions the MODEL reads, not interface copy
      // a reader ever sees.
      if (f === 'manifest.ts') continue;
      const src = readFileSync(join(SRC_DIR, f), 'utf8');
      src.split('\n').forEach((line, i) => {
        const code = line.replace(/\/\/.*$/, '').replace(/^\s*\*.*$/, '');
        if (/['"`][^'"`]*[一-鿿][^'"`]*['"`]/.test(code)) {
          offenders.push(`${f}:${i + 1}: ${line.trim()}`);
        }
      });
    }
    expect(offenders).toEqual([]);
  });
});
