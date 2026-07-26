import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Local dev: Vite serves the app on :5173 and proxies /api to the Go backend
// (same-origin from the browser's perspective, so the dc_sid cookie works).
// Production: `npm run build` emits dist/, which the Go server itself serves
// (STATIC_DIR) — nginx in front is optional.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.DAYCORE_API || 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 900,
  },
});
