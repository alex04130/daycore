import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// The gate that keeps the language packs honest.
//
// ⚠️ Without something like this, i18n rots in exactly one direction: somebody
// adds a string, writes the zh-CN value, and the en-US pack silently grows a
// hole that renders as a bare key on a reader's screen. The backend has the
// same gate (make check-i18n) for the same reason.
//
// It is a TEST rather than a script so it runs in `npm run build` — a check
// nobody remembers to run is a check that does not exist.

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

function sources(): string[] {
  return readdirSync(SRC_DIR)
    .filter((f) => (f.endsWith('.ts') || f.endsWith('.tsx')) && !f.endsWith('.test.ts'))
    .map((f) => readFileSync(join(SRC_DIR, f), 'utf8'));
}

/**
 * Every key the source asks for.
 *
 * ⚠️ Scans the WHOLE `t(...)` call, not just its first argument. The obvious
 * regex — `t\(\s*'key'` — misses every key chosen by a ternary
 * (`t(x ? 'a.b' : 'c.d')`), which is the natural way to write two-state copy
 * and which 汀 uses four times. The first version of this gate reported those
 * four as orphans: a check that is wrong in the direction of "delete this
 * translation" is worse than no check.
 */
function usedKeys(): Set<string> {
  const keys = new Set<string>();
  // ⚠️ Dotted only. Scanning the whole call also picks up the values a ternary
  // COMPARES against — `t(p.level === 'L3' ? … )` yields 'L3' — and reporting
  // those as undefined keys is a check that cries wolf. Every key here is
  // dotted, and the test below enforces that so this discriminator cannot
  // silently stop working.
  const literal = /['"]([a-z][\w]*(?:\.[\w]+)+)['"]/g;
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
      for (const m of src.slice(i, end).matchAll(literal)) keys.add(m[1]!);
    }
  }
  return keys;
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

  // ⚠️ Load-bearing for usedKeys above, which tells a key from a comparison
  // value by the dot. A flat key would be invisible to the gate — so the
  // convention is asserted rather than assumed.
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

  // ⚠️ Both directions. A key the source asks for and no pack has renders as a
  // bare key; a key a pack has and nothing asks for is a translator's wasted
  // work and a reviewer's confusion about what a screen still says.
  it('defines every key the source asks for', () => {
    const defined = new Set(Object.keys(all['zh-CN']!));
    const undefinedKeys = [...usedKeys()].filter((k) => !defined.has(k)).sort();
    expect(undefinedKeys).toEqual([]);
  });

  it('has nothing nobody asks for', () => {
    const used = usedKeys();
    const orphans = Object.keys(all['zh-CN']!)
      .filter((k) => !used.has(k))
      .sort();
    expect(orphans).toEqual([]);
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
  //
  // In 汀 the last one to hide was the undo bar's label, in store.ts rather than in a
  // component. That is where this kind of thing survives a manual pass.
  it('has no CJK string literals outside comments', () => {
    const files = readdirSync(SRC_DIR).filter(
      (f) => (f.endsWith('.ts') || f.endsWith('.tsx')) && !f.endsWith('.test.ts'),
    );
    const offenders: string[] = [];
    for (const f of files) {
      // manifest.ts is the exception, and a real one: its strings are 汀's own
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
