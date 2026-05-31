import { defineConfig } from "tsup"

export default defineConfig({
  entry: {
    index: "src/index.ts",
  },
  format: ["esm"],
  platform: "node",
  target: "node25",
  outDir: "dist",
  outExtension: () => ({ js: ".js" }),
  clean: true,
  splitting: false,
  sourcemap: true,
  noExternal: ["@tokeninsights/logger"],
})
