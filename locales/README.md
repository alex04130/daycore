# Message packs

Drop `<locale>.json` files here to add or override a language **without
rebuilding**. The server reads this directory at startup (`LOCALES_DIR`,
default `locales`).

Only `zh-CN` and `en-US` are compiled into the binary, and they are a *floor* —
enough to render a page and explain itself when there is no database and no
files. They are not the set of languages the product supports.

## Three layers, and this is the middle one

```
database overrides    console-editable, shared across instances   ← wins
files here            LOCALES_DIR/<locale>.json
embedded              Go literals, zh-CN and en-US only           ← floor
```

Resolution goes **locale by locale, not layer by layer**: for each candidate
language in the fallback chain, all three layers are asked before moving to the
next language. Done the other way round, a half-finished French override in the
database would shadow a complete English translation underneath it.

The database layer is loaded at startup by `server.ReloadLocaleOverrides` and
again after a console edit. It was wired on 2026-07-29 — before that the layer
existed in the catalog and in both stores but nothing connected them, so a
running server had two layers while three documents described three.

A language that exists **only** in the database still shows up in
`GET /api/version`'s `locales.available`, and deleting it there removes it. So
"install a language" and "uninstall a language" are both data operations.

## Format

A flat object, message key → text:

```json
{
  "mood.happy": "うれしい",
  "category.diet": "食事",
  "date.weekday.0": "日曜日"
}
```

The filename is the locale tag: `ja-JP.json`, `de-DE.json`, `zh-TW.json`.
Region variants are matched before another language is tried, so a `zh-TW.json`
that omits a key falls back to `zh-CN` rather than to English.

## Starting a translation

Export the full key list from the admin console (or `Catalog.Export`), translate
the values, save the result here. The export always lists **every** key — ones
you have not translated carry their fallback text, so the file doubles as a
worklist.

Keys you leave out are not an error; the fallback chain covers them and the
console reports coverage per language.

## Keys that carry format verbs

A few messages interpolate values and **must keep the same verbs in the same
order**:

| Key | Verbs |
|---|---|
| `weather.precip` | `%d` — precipitation probability |
| `inbox.categoryLine` | `%s` id, `%s` name, `%s` hint |
| `date.row` | `%s` term, `%s` date, `%s` weekday |
| `date.rel` | `%s` week qualifier, `%s` short weekday |
| `worker.replanUser` | `%s` — overdue-block JSON |
| `worker.deadline.overdue` / `.alsoSoon` / `.onlySoon` | `%d` — a count |
| `worker.deadline.overdueTag` | `%s` — a formatted due time |
| `worker.deadline.item` | `%d` index, `%s` title, `%s` due |

Punctuation and spacing are part of the translation, not something the server
adds: `date.row` is `| %s | %s（%s） |` in Chinese and `| %s | %s (%s) |` in
English, and `date.rel` is `%s%s` in Chinese but `%s %s` in English because
`本周` runs onto `三` while "Next" does not run onto "Monday".

## What is not here

**Prompt templates** are a separate mechanism — `internal/ai/prompts/<locale>/`,
embedded, with a database override table behind the admin console. A missing
prompt fails startup rather than falling back, because a fallback chain cannot
paper over an instruction the model needs.

So a language installed only through this directory gets translated UI text and
English prompts. That degrades sensibly, but it is a partial translation; the
console shows which is which.
