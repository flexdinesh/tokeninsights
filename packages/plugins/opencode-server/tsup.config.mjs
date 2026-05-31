import { defineConfig } from "tsup"

export default defineConfig({
  entry: {
    index: "src/index.ts",
    "oc-tokeninsights-writer": "src/oc-tokeninsights-writer.ts",
  },
  format: ["esm"],
  platform: "node",
  target: "node24",
  outDir: "dist",
  clean: true,
  splitting: false,
  sourcemap: true,
  external: ["better-sqlite3"],
  noExternal: ["@tokeninsights/logger"],
})
