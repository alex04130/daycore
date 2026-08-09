import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The operations console.
//
// # Why it builds INTO the Go resource tree
//
// outDir points at internal/resources/data/console, which is embedded in the
// binary. The console is version-locked to this backend — it renders this
// build's config classification, this build's provider fields, this build's
// prompt keys — and it is needed exactly when things are broken, which is the
// case where "deploy a second thing first" is the worst possible requirement.
//
// A committed placeholder index.html lives there so `go build ./...` works on a
// fresh clone before anybody runs npm. This build overwrites it.
//
// # base: /admin/
//
// The Go server serves it under /admin, so asset URLs have to be written that
// way at build time — a console whose assets 404 is indistinguishable from one
// that was never built, and that is a bad half-hour to hand somebody.
export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: {
    outDir: '../../internal/resources/data/console',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: process.env.DAYCORE_API || 'http://localhost:8080',
        // Same-origin from the browser's point of view, so the httpOnly admin
        // cookie is sent. changeOrigin would break the Origin check that
        // protects the cookie path.
        changeOrigin: false,
      },
    },
  },
});
