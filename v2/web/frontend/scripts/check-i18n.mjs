#!/usr/bin/env node
// Checks that the zh-CN and en-US dictionaries in src/i18n.js share the same key set.
// Prefers importing the module directly; falls back to static text parsing if Node
// cannot load it (e.g. a future browser-only dependency sneaks into module scope).

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const I18N_PATH = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src/i18n.js');
const LANGS = ['zh-CN', 'en-US'];

async function keysViaImport() {
  const mod = await import(pathToFileURL(I18N_PATH).href);
  const dict = mod.dict;
  if (!dict) throw new Error('i18n.js does not export dict');
  return LANGS.map((l) => {
    if (!dict[l]) throw new Error(`dict has no "${l}" block`);
    return Object.keys(dict[l]);
  });
}

// Extracts top-level keys of the object literal whose opening brace is at src[openIdx].
// Skips comments and string contents; handles quoted and bare keys; treats [] like {}
// for nesting so commas inside array values don't confuse key detection.
function extractKeys(src, openIdx) {
  const keys = [];
  let i = openIdx + 1;
  let depth = 1;
  let expectKey = true;
  while (i < src.length && depth > 0) {
    const ch = src[i];
    if (ch === '/' && src[i + 1] === '/') {
      const nl = src.indexOf('\n', i);
      if (nl < 0) break;
      i = nl + 1;
    } else if (ch === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2);
      if (end < 0) break;
      i = end + 2;
    } else if (ch === "'" || ch === '"' || ch === '`') {
      const quote = ch;
      const start = ++i;
      while (i < src.length && src[i] !== quote) {
        if (src[i] === '\\') i++;
        i++;
      }
      const text = src.slice(start, i);
      i++;
      if (depth === 1 && expectKey) {
        let j = i;
        while (j < src.length && /\s/.test(src[j])) j++;
        if (src[j] === ':') {
          keys.push(text.replace(/\\(.)/g, '$1'));
          expectKey = false;
        }
      }
    } else if (ch === '{' || ch === '[') {
      depth++;
      i++;
    } else if (ch === '}' || ch === ']') {
      depth--;
      i++;
    } else if (ch === ',') {
      if (depth === 1) expectKey = true;
      i++;
    } else if (depth === 1 && expectKey && /[A-Za-z_$]/.test(ch)) {
      const start = i;
      while (i < src.length && /[A-Za-z0-9_$]/.test(src[i])) i++;
      let j = i;
      while (j < src.length && /\s/.test(src[j])) j++;
      if (src[j] === ':') {
        keys.push(src.slice(start, i));
        expectKey = false;
      }
    } else {
      i++;
    }
  }
  return keys;
}

function keysViaParse() {
  const src = fs.readFileSync(I18N_PATH, 'utf8');
  return LANGS.map((l) => {
    const m = src.match(new RegExp(`['"]${l}['"]\\s*:\\s*\\{`));
    if (!m) throw new Error(`cannot locate "${l}" block in ${I18N_PATH}`);
    return extractKeys(src, m.index + m[0].length - 1);
  });
}

let keySets;
try {
  keySets = await keysViaImport();
} catch (err) {
  console.error(`[check-i18n] import failed (${err.message}), falling back to static parsing`);
  keySets = keysViaParse();
}

const [zhKeys, enKeys] = keySets;
const zhSet = new Set(zhKeys);
const enSet = new Set(enKeys);
const missing = [
  [LANGS[1], zhKeys.filter((k) => !enSet.has(k))],
  [LANGS[0], enKeys.filter((k) => !zhSet.has(k))],
];

let failed = false;
for (const [lang, list] of missing) {
  if (!list.length) continue;
  failed = true;
  console.error(`Missing in ${lang} (${list.length}):`);
  for (const k of list) console.error(`  ${k}`);
}

if (failed) process.exit(1);
console.log(`OK — ${zhSet.size} keys aligned across ${LANGS.join(' / ')}`);
