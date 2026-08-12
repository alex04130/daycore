import { beforeEach, describe, expect, it, vi } from 'vitest';
import { chainFor, loadCatalog, preferredLocale, chooseLocale } from './i18n';

// The four rules from docs/specs/frontend-manifest.md「前端的多语言形态」.
// Each test names the one it pins.

const PACKS: Record<string, Record<string, string>> = {
  'zh-CN': { hello: '你好', only_zh: '只有中文', greet: '你好，{name}' },
  'en-US': { hello: 'Hello', only_en: 'English only', greet: 'Hello, {name}' },
  'zh-TW': { hello: '妳好' },
};

beforeEach(() => {
  chooseLocale(null);
  vi.stubGlobal('document', { baseURI: 'https://ting.example/' });
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u: string) => {
      const m = /locales\/([\w-]+)\.json/.exec(String(u));
      const pack = m && PACKS[m[1]!];
      return pack
        ? ({ ok: true, json: async () => pack } as Response)
        : ({ ok: false, json: async () => ({}) } as Response);
    }),
  );
});

// ③ exact → same base language → default → any.
describe('rule ③ · the fallback chain matches the backend', () => {
  it('prefers exact, then the same base language, then the default', () => {
    expect(chainFor('zh-TW', ['en-US', 'zh-CN', 'zh-TW'], 'en-US')).toEqual(['zh-TW', 'zh-CN', 'en-US']);
    expect(chainFor('zh-HK', ['en-US', 'zh-CN'], 'en-US')).toEqual(['zh-CN', 'en-US']);
  });

  // ⚠️ The last step is the one that looks wrong and is not. A half-translated
  // interface in a language the reader may not know beats blanks: a missing
  // translation is a CONTENT gap, a blank screen reads as a RENDERING FAULT.
  it('falls through to ANY available locale rather than to nothing', () => {
    const chain = chainFor('fr-FR', ['ja-JP', 'ko-KR'], 'de-DE');
    expect(chain.length).toBeGreaterThan(0);
    expect(chain).toEqual(['ja-JP', 'ko-KR']);
  });

  it('never repeats a locale, so a pack is never loaded twice', () => {
    const chain = chainFor('zh-CN', ['zh-CN', 'en-US'], 'zh-CN');
    expect(new Set(chain).size).toBe(chain.length);
  });
});

describe('rule ③ · a value missing from the first pack comes from the next', () => {
  it('reads through the chain rather than stopping at the top', async () => {
    const c = await loadCatalog('zh-TW', ['zh-TW', 'zh-CN', 'en-US'], 'en-US');
    expect(c.t('hello')).toBe('妳好'); // exact wins
    expect(c.t('only_zh')).toBe('只有中文'); // falls to zh-CN
    expect(c.t('only_en')).toBe('English only'); // falls all the way to en-US
  });

  it('survives a locale whose pack does not exist', async () => {
    const c = await loadCatalog('xx-YY', ['xx-YY', 'en-US'], 'en-US');
    expect(c.t('hello')).toBe('Hello');
    expect(c.chain).toEqual(['en-US']);
  });
});

// ④ A missing key shows the KEY.
describe('rule ④ · a missing key renders as the key', () => {
  it('is neither blank nor invented', async () => {
    const c = await loadCatalog('en-US', ['en-US'], 'en-US');
    // Not "" — that reads as a layout bug. Not a guess — that reads as a
    // mistranslation. The key says "nobody has written this yet", and which.
    expect(c.t('nobody.wrote.this')).toBe('nobody.wrote.this');
  });

  it('treats an EMPTY translation as missing too', async () => {
    // A translator who left a value blank has not translated it; rendering ""
    // would put their blank on screen and look like the layout bug above.
    (PACKS['en-US'] as Record<string, string>)['blank'] = '';
    (PACKS['zh-CN'] as Record<string, string>)['blank'] = '有内容';
    const c = await loadCatalog('en-US', ['en-US', 'zh-CN'], 'zh-CN');
    expect(c.t('blank')).toBe('有内容');
  });
});

describe('interpolation', () => {
  it('substitutes named placeholders', async () => {
    const c = await loadCatalog('zh-CN', ['zh-CN'], 'zh-CN');
    expect(c.t('greet', { name: '小禾' })).toBe('你好，小禾');
  });

  // ⚠️ An unpassed placeholder stays visible. `{n} 分钟` rendering as ` 分钟`
  // looks like a layout bug; leaving `{n}` looks like what it is.
  it('leaves an unpassed placeholder in place rather than emptying it', async () => {
    const c = await loadCatalog('zh-CN', ['zh-CN'], 'zh-CN');
    expect(c.t('greet', { other: 'x' })).toBe('你好，{name}');
  });
});

// ① Copy is DATA: a built artifact gains a language by gaining a FILE.
describe('rule ① · a language is a file, not a release', () => {
  it('loads a locale this build has never heard of', async () => {
    // de-DE appears nowhere in 汀's source. An operator dropped de-DE.json into
    // the deployment's LOCALES_DIR and next to the built assets; nothing was
    // rebuilt. If the loader had a compile-time list this would be impossible,
    // which is exactly the failure rule ① names.
    PACKS['de-DE'] = { hello: 'Hallo' };
    const c = await loadCatalog('de-DE', ['de-DE', 'en-US'], 'en-US');
    expect(c.locale).toBe('de-DE');
    expect(c.t('hello')).toBe('Hallo');
    // …and it still falls through for what that pack does not have.
    expect(c.t('only_en')).toBe('English only');
  });
});

// ② The list of languages comes from the backend.
describe('rule ② · available languages come from the deployment', () => {
  it('matches the browser preference against what the DEPLOYMENT has', () => {
    // The browser wants Japanese; this deployment does not have it. 汀 must not
    // pick it just because it knows the tag.
    vi.stubGlobal('navigator', { languages: ['ja-JP', 'zh-CN'] });
    expect(preferredLocale(['en-US', 'zh-CN'], 'en-US')).toBe('zh-CN');
  });

  it('offers a language 汀 has never heard of, if the deployment has it', () => {
    // An operator dropped de-DE.json into LOCALES_DIR. Nothing in 汀 mentions
    // German, and it still has to be reachable — that is the whole rule.
    vi.stubGlobal('navigator', { languages: ['de-DE'] });
    expect(preferredLocale(['en-US', 'de-DE'], 'en-US')).toBe('de-DE');
  });

  it("honours the reader's own choice over the browser's", () => {
    vi.stubGlobal('navigator', { languages: ['en-US'] });
    chooseLocale('zh-CN');
    expect(preferredLocale(['en-US', 'zh-CN'], 'en-US')).toBe('zh-CN');
  });

  // ⚠️ Storage can be absent — private browsing, an embedded webview. The
  // choice must still TAKE EFFECT for this page load; only its persistence is
  // allowed to degrade. The first version wrote only to localStorage and
  // swallowed the failure, so in private mode picking a language did nothing at
  // all and the setting screen showed no reason why.
  it("takes effect even when there is no storage to remember it in", () => {
    vi.stubGlobal('navigator', { languages: ['en-US'] });
    vi.stubGlobal('localStorage', {
      getItem() { throw new Error('denied'); },
      setItem() { throw new Error('denied'); },
      removeItem() { throw new Error('denied'); },
    });
    chooseLocale('zh-CN');
    expect(preferredLocale(['en-US', 'zh-CN'], 'en-US')).toBe('zh-CN');
  });

  it('falls back to the deployment default when nothing matches', () => {
    vi.stubGlobal('navigator', { languages: ['fr-FR'] });
    expect(preferredLocale(['en-US', 'zh-CN'], 'zh-CN')).toBe('zh-CN');
  });
});
