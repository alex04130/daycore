/// <reference types="vite/client" />

// Injected by vite.config.ts from package.json's version. Declared here so the
// manifest can report it without a runtime import of package.json — which would
// pull the whole file, including devDependencies, into the bundle.
declare const __APP_VERSION__: string;
