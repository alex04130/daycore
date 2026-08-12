// ⚠️ defineConfig from vitest/config, not from vite — it is vite's own, widened
// to know about the `test` block below. With vite's, `tsc --noEmit` rejects the
// whole config object and the error points at the last overload rather than at
// the field.
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import pkg from './package.json';

// 琉璃初版 · 页面制 — the fourth and last of them.
//
// # ⚠️ This does NOT build into the Go binary
//
// web/console/vite.config.js points outDir at internal/resources/data/console
// because the console is version-locked to its backend and is needed exactly
// when things are broken. The four user-facing frontends are the opposite on
// both counts: each negotiates with its backend at runtime (POST /api/version),
// and each is planned to become its own git submodule with its own release
// cadence (docs/ROADMAP.md 阶段 κ). Embedding one would re-tie the knot that
// separation exists to cut.
//
// So: a plain static build. Serve dist/ from anywhere.
//
// # base is './'
//
// Relative, because this build does not know its own URL. The console can hard
// code /admin/ since the Go server serves it there; this may be at a domain
// root, a subpath, a CDN, or a file:// bundle inside a Capacitor shell. An
// absolute base would be a guess, and a wrong guess is assets that 404 — which
// looks exactly like a build that never happened.

export default defineConfig({
  plugins: [react()],
  base: './',
  // The version reported in the handshake. One source — package.json — so the
  // number on the console's screen is the number that was built.
  define: { __APP_VERSION__: JSON.stringify(pkg.version) },
  build: { outDir: 'dist', emptyOutDir: true },

  // ⚠️ The test zone is PINNED, and the choice is not arbitrary.
  //
  // This is the only one of the four frontends that does date ARITHMETIC (see
  // src/days.ts — the other three hard-code today and never move off it), and
  // date arithmetic has two classic bugs that are each invisible in the wrong
  // zone:
  //
  //   `new Date('2026-08-11')` parses as UTC     → wrong day only WEST of
  //                                                 Greenwich
  //   whole-day differences via Math.floor        → wrong only in a zone that
  //                                                 observes DST
  //
  // America/Chicago is both: UTC-6/-5, and it springs forward. Under it, both
  // mutations fail the suite. Under UTC — which is what a container defaults to
  // — neither does, and the assertions quietly become decoration while still
  // reporting green.
  //
  // The cost, stated: one process holds one zone, so pinning means these tests
  // never run under the reader's own. That is the right trade — the module's
  // job is to be correct in EVERY zone, and a suite that only exercises whatever
  // the machine happens to be set to is testing the machine.
  test: { env: { TZ: 'America/Chicago' } },
  server: {
    port: 5178,
    proxy: {
      // Dev only. In production the backend address is configured at runtime on
      // the /setting screen — see packages/core/src/backend.ts for why a
      // build-time constant would make a self-hosted deployment impossible.
      '/api': {
        target: process.env.DAYCORE_API || 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
});
