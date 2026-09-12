import { spawn } from "node:child_process"
import { DatabaseSync } from "node:sqlite"
import { copyFile, mkdir, readFile, readdir, rm } from "node:fs/promises"
import { basename, dirname, join, relative, resolve } from "node:path"
import { fileURLToPath } from "node:url"

import { workspaceRoot } from "./workspace.mjs"

const scriptPath = fileURLToPath(import.meta.url)
const fixtureRelativePath = join(
  "packages",
  "cli",
  "testdata",
  "conformance",
  "sync-first-basic",
  "source",
)

export async function setupDevData({
  workspaceRoot: configuredWorkspaceRoot = workspaceRoot,
  sync = runSync,
} = {}) {
  const root = resolve(configuredWorkspaceRoot)
  const outputDir = join(root, ".tokeninsights-dev")
  assertControlledOutput(root, outputDir)

  const fixtureDir = join(root, fixtureRelativePath)
  const sourceDir = join(outputDir, "source")
  const dbPath = join(outputDir, "tokeninsights.sqlite")

  await rm(outputDir, { recursive: true, force: true })
  await mkdir(sourceDir, { recursive: true })
  await copyJSONLFixtures(fixtureDir, sourceDir)
  await materializeOpenCode(fixtureDir, sourceDir)
  await sync({
    binaryPath: join(root, "packages", "cli", "bin", "tokeninsights"),
    dbPath,
    sourceDir,
  })

  return { dbPath, outputDir, sourceDir }
}

function assertControlledOutput(configuredWorkspaceRoot, outputDir) {
  if (
    basename(outputDir) !== ".tokeninsights-dev" ||
    dirname(outputDir) !== configuredWorkspaceRoot
  ) {
    throw new Error(`refusing to recreate uncontrolled directory: ${outputDir}`)
  }
}

async function copyJSONLFixtures(fixtureDir, sourceDir) {
  for (const path of await listFiles(fixtureDir)) {
    if (!path.endsWith(".jsonl")) {
      continue
    }
    const destination = join(sourceDir, relative(fixtureDir, path))
    await mkdir(dirname(destination), { recursive: true })
    await copyFile(path, destination)
  }
}

async function listFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...(await listFiles(path)))
    } else if (entry.isFile()) {
      files.push(path)
    }
  }
  return files
}

async function materializeOpenCode(fixtureDir, sourceDir) {
  const sqlPath = join(fixtureDir, "opencode", "source.sql")
  const dbPath = join(sourceDir, "opencode", "opencode.db")
  await mkdir(dirname(dbPath), { recursive: true })
  const database = new DatabaseSync(dbPath)
  try {
    database.exec(await readFile(sqlPath, "utf8"))
  } finally {
    database.close()
  }
}

async function runSync({ binaryPath, dbPath, sourceDir }) {
  await new Promise((resolveRun, rejectRun) => {
    const child = spawn(
      binaryPath,
      ["sync", "--all", "--source-dir", sourceDir, "--db-path", dbPath],
      { stdio: "inherit" },
    )
    child.once("error", rejectRun)
    child.once("exit", (code, signal) => {
      if (code === 0) {
        resolveRun()
        return
      }
      rejectRun(
        new Error(
          signal === null
            ? `tokeninsights sync exited with code ${code}`
            : `tokeninsights sync exited with signal ${signal}`,
        ),
      )
    })
  })
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  const { dbPath } = await setupDevData()
  console.log(`development data ready: ${dbPath}`)
}
