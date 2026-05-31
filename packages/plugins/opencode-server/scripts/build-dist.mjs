import { copyFile, mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const packageRoot = dirname(fileURLToPath(new URL("../package.json", import.meta.url)))
const distDir = join(packageRoot, "dist")
const sourcePackagePath = join(packageRoot, "package.json")
const schemaSourcePath = join(packageRoot, "..", "..", "schema", "schema.sql")
const distPackagePath = join(distDir, "package.json")
const distSchemaPath = join(distDir, "schema.sql")

const sourcePackage = JSON.parse(await readFile(sourcePackagePath, "utf8"))

await mkdir(distDir, { recursive: true })
await copyFile(schemaSourcePath, distSchemaPath)

const distPackage = {
  name: sourcePackage.name,
  version: sourcePackage.version,
  description: sourcePackage.description,
  type: "module",
  main: "./index.js",
  exports: "./index.js",
  engines: sourcePackage.engines,
  dependencies: {
    "better-sqlite3": sourcePackage.dependencies["better-sqlite3"],
  },
  peerDependencies: sourcePackage.peerDependencies,
}

await writeFile(distPackagePath, `${JSON.stringify(distPackage, null, 2)}\n`, "utf8")
