import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["src/index.ts"],
  format: ["esm"],
  dts: true,
  sourcemap: false,
  clean: true,
  treeshake: true,
  // React (and react-dom) are externalized so the design-sync converter can
  // load them from its shared _vendor/ runtime. All other deps (framer-motion,
  // lucide-react, clsx, tailwind-merge) are bundled by the converter's esbuild
  // pass from this package's node_modules, so leaving them external here is fine.
  external: ["react", "react-dom", "react/jsx-runtime"],
  outExtension() {
    return { js: ".js" };
  },
});
