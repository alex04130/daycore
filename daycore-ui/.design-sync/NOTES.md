# Daycore Design System — design-sync notes

Repo-specific gotchas for future syncs of `@daycore/ui` (the design system
extracted from the Daycore Next.js app).

## Build / pipeline

- The DS package lives in `daycore-ui/` (its own package, not part of the Next
  app's build). Run all design-sync commands from `daycore-ui/`.
- `buildCmd` is `npm run build` = `tsup` (ESM JS + `.d.ts`) **then** the Tailwind
  v4 CLI (`tailwindcss -i src/styles.css -o dist/styles.css --minify`). Both must
  run; `cfg.cssEntry` points at the compiled `dist/styles.css`.
- `node` v24 + `npm` only (no `bun` in this env). Converter deps + playwright
  installed under `.ds-sync/`.
- Render check: a chromium **build 1223** is cached at `~/.cache/ms-playwright/`,
  which is pinned by **playwright 1.60.0** — install that exact version into
  `.ds-sync/` (with `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1`) so the cached browser is
  reused with no ~200MB download.

## Previews

- **`src/styles.css` `@source` includes `../.design-sync/previews/**`** on purpose:
  Tailwind only compiles utilities it sees, and preview wrappers use layout
  utilities (`grid`, `grid-cols-*`, `w-72/80`, `gap-2.5`) that no component source
  uses. Without this line those utilities are missing and previews like the
  MoodTile grid collapse to a single column. Keep it.
- Previews may `import` from `'@daycore/ui'` AND from `'lucide-react'` directly —
  both resolve in the preview build.
- Realistic Chinese content + emoji render fine in the cached chromium.

## Re-sync risks

- The shipped `dist/styles.css` size depends on which utilities the preview files
  use (because of the `@source` line above). Adding a preview that uses a new
  utility requires a CSS rebuild (`npm run build`) before re-running the converter,
  or that utility won't ship. The standard `buildCmd` handles this — just always
  rebuild before the converter on re-sync.
- All components currently land in the `general` group (no per-component docs).
  Add a `docs/` tree with `category:` frontmatter (and set `cfg.docsDir`) to group
  the DS pane and enrich the synthesized `.prompt.md`s — optional polish, deferred.
