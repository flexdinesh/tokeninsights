import assert from "node:assert/strict"
import { DatabaseSync } from "node:sqlite"
import { cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises"
import { dirname, join, resolve } from "node:path"
import test from "node:test"

import { setupDevData } from "../src/setup-dev-data.mjs"
import { workspacePath } from "../src/workspace.mjs"

const fixtureRelativePath = join(
  "packages",
  "cli",
  "testdata",
  "conformance",
  "sync-first-basic",
  "source",
)

test("materializes deterministic development sources and removes stale output", async () => {
  const configuredWorkspaceRoot = await createWorkspace()
  const outputDir = resolve(configuredWorkspaceRoot, ".tokeninsights-dev")
  const stalePath = join(outputDir, "stale.txt")
  await mkdir(outputDir, { recursive: true })
  await writeFile(stalePath, "stale")

  const syncCalls = []
  const sync = async (paths) => {
    syncCalls.push(paths)
    const sourceDatabase = new DatabaseSync(
      join(paths.sourceDir, "opencode", "opencode.db"),
      { readOnly: true },
    )
    try {
      const tables = sourceDatabase
        .prepare("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
        .all()
        .map((row) => row.name)
      assert.ok(tables.includes("message"))
    } finally {
      sourceDatabase.close()
    }

    const outputDatabase = new DatabaseSync(paths.dbPath)
    outputDatabase.exec("CREATE TABLE marker (value TEXT NOT NULL); INSERT INTO marker VALUES ('ready');")
    outputDatabase.close()
  }

  const first = await setupDevData({ workspaceRoot: configuredWorkspaceRoot, sync })
  assert.equal(first.outputDir, outputDir)
  await assert.rejects(readFile(stalePath), { code: "ENOENT" })
  const firstFiles = await relativeFiles(outputDir)
  const fixtureFiles = await relativeFiles(join(configuredWorkspaceRoot, fixtureRelativePath))
  const expectedFiles = [
    ...fixtureFiles.filter((path) => path.endsWith(".jsonl")).map((path) => join("source", path)),
    join("source", "opencode", "opencode.db"),
    "tokeninsights.sqlite",
  ].sort()
  assert.deepEqual(firstFiles, expectedFiles)

  await writeFile(join(outputDir, "stale-again.txt"), "stale")
  const second = await setupDevData({ workspaceRoot: configuredWorkspaceRoot, sync })
  const secondFiles = await relativeFiles(outputDir)

  assert.deepEqual(second, first)
  assert.deepEqual(secondFiles, firstFiles)
  assert.equal(syncCalls.length, 2)
  assert.deepEqual(syncCalls[0], syncCalls[1])
  const outputDatabase = new DatabaseSync(first.dbPath, { readOnly: true })
  try {
    assert.equal(outputDatabase.prepare("SELECT value FROM marker").get().value, "ready")
  } finally {
    outputDatabase.close()
  }
})

async function createWorkspace() {
  const testRoot = workspacePath(".scratch", "test-tmp")
  await mkdir(testRoot, { recursive: true })
  const configuredWorkspaceRoot = await mkdtemp(join(testRoot, "tokeninsights-dev-data-"))
  test.after(async () => {
    await rm(configuredWorkspaceRoot, { recursive: true, force: true })
  })

  const source = workspacePath(fixtureRelativePath)
  const destination = join(configuredWorkspaceRoot, fixtureRelativePath)
  await mkdir(dirname(destination), { recursive: true })
  await cp(source, destination, { recursive: true })
  return configuredWorkspaceRoot
}

async function relativeFiles(root) {
  const entries = await readdir(root, { recursive: true, withFileTypes: true })
  return entries
    .filter((entry) => entry.isFile())
    .map((entry) => join(entry.parentPath, entry.name).slice(root.length + 1))
    .sort()
}
