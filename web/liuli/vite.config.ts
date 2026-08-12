import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 琉璃 · 长卷 — the third of the four.
//
// # ⚠️ Unlike the console, this does NOT build into the Go binary
//
// web/console/vite.config.js points outDir at internal/resources/data/console
// because the console is version-locked to its backend and is needed exactly
// when things are broken. 汀 is the opposite on both counts: it talks to a
// backend it negotiates with at runtime (POST /api/version), and it is planned
// to become its own git submodule with its own release cadence
// (docs/ROADMAP.md 阶段 κ). Embedding it would re-tie the knot that separation
// exists to cut.
//
// So: a plain static build. Serve dist/ from anywhere.
//
// # base is './'
//
// Relative, because this build does not know its own URL. The console can hard
// code /admin/ since the Go server serves it there; 汀 may be at a domain root,
// a subpath, a CDN, or a file:// bundle inside a Capacitor shell. An absolute
// base would be a guess, and a wrong guess is assets that 404 — which looks
// exactly like a build that never happened.
import pkg from './package.json';

export default defineConfig({
  plugins: [react()],
  base: './',
  // The version 汀 reports in its handshake. One source — package.json — so the
  // number on the console's screen is the number that was built.
  define: { __APP_VERSION__: JSON.stringify(pkg.version) },
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5177,
    proxy: {
      // Dev only. In production the backend address is configured at runtime on
      // the /setting screen — see src/backend.ts for why a build-time constant
      // would make a self-hosted deployment impossible.
      '/api': {
        target: process.env.DAYCORE_API || 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
});
