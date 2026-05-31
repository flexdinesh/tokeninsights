import { mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"

const packageRoot = dirname(fileURLToPath(new URL("../package.json", import.meta.url)))
const distDir = join(packageRoot, "dist")
const sourcePackagePath = join(packageRoot, "package.json")
const distPackagePath = join(distDir, "package.json")

const sourcePackage = JSON.parse(await readFile(sourcePackagePath, "utf8"))

await mkdir(distDir, { recursive: true })

const distPackage = {
  name: sourcePackage.name,
  version: sourcePackage.version,
  description: sourcePackage.description,
  type: "module",
  main: "./index.js",
  exports: "./index.js",
  pi: {
    extensions: ["./index.js"],
  },
  engines: sourcePackage.engines,
}

await writeFile(distPackagePath, `${JSON.stringify(distPackage, null, 2)}\n`, "utf8")
