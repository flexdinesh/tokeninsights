import assert from 'node:assert/strict'
import { DatabaseSync } from 'node:sqlite'
import { cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import test from 'node:test'

import { setupDevData } from '../src/setup-dev-data.ts'
import type { SyncFunction, SyncPaths } from '../src/setup-dev-data.ts'
import { workspacePath } from '../src/workspace.ts'

const fixtureRelativePath = join(
  'packages',
  'cli',
  'testdata',
  'conformance',
  'sync-first-basic',
  'source',
)

void test('materializes deterministic development sources and removes stale output', async () => {
  const configuredWorkspaceRoot = await createWorkspace()
  const outputDir = resolve(configuredWorkspaceRoot, '.tokeninsights-dev')
  const stalePath = join(outputDir, 'stale.txt')
  await mkdir(outputDir, { recursive: true })
  await writeFile(stalePath, 'stale')

  const syncCalls: SyncPaths[] = []
  const sync: SyncFunction = async (paths) => {
    syncCalls.push(paths)
    const sourceDatabase = new DatabaseSync(join(paths.sourceDir, 'opencode', 'opencode.db'), {
      readOnly: true,
    })
    try {
      const tables = sourceDatabase
        .prepare("SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")
        .all()
        .map((row) => row.name)
      assert.ok(tables.includes('message'))
    } finally {
      sourceDatabase.close()
    }

    const collectorDatabase = new DatabaseSync(paths.collectorDBPath)
    collectorDatabase.exec(
      "CREATE TABLE raw_marker (value TEXT NOT NULL); INSERT INTO raw_marker VALUES ('local');",
    )
    collectorDatabase.close()
    await writeFile(paths.serverDBPath, 'mock-ready')
  }

  const first = await setupDevData({
    workspaceRoot: configuredWorkspaceRoot,
    sync,
    prepare: async () => {
      await rm(outputDir, { recursive: true, force: true })
    },
  })
  assert.equal(first.outputDir, outputDir)
  await assert.rejects(readFile(stalePath), { code: 'ENOENT' })
  const firstFiles = await relativeFiles(outputDir)
  const fixtureFiles = await relativeFiles(join(configuredWorkspaceRoot, fixtureRelativePath))
  const expectedFiles = [
    ...fixtureFiles.filter((path) => path.endsWith('.jsonl')).map((path) => join('source', path)),
    join('source', 'opencode', 'opencode.db'),
    'collector.sqlite',
    'server.duckdb',
  ].toSorted()
  assert.deepEqual(firstFiles, expectedFiles)

  await writeFile(join(outputDir, 'stale-again.txt'), 'stale')
  const second = await setupDevData({
    workspaceRoot: configuredWorkspaceRoot,
    sync,
    prepare: async () => {
      await rm(outputDir, { recursive: true, force: true })
    },
  })
  const secondFiles = await relativeFiles(outputDir)

  assert.deepEqual(second, first)
  assert.deepEqual(secondFiles, firstFiles)
  assert.equal(syncCalls.length, 2)
  assert.deepEqual(syncCalls[0], syncCalls[1])
  assert.notEqual(first.collectorDBPath, first.serverDBPath)
  assert.equal(await readFile(first.serverDBPath, 'utf8'), 'mock-ready')
})

async function createWorkspace(): Promise<string> {
  const testRoot = workspacePath('.scratch', 'test-tmp')
  await mkdir(testRoot, { recursive: true })
  const configuredWorkspaceRoot = await mkdtemp(join(testRoot, 'tokeninsights-dev-data-'))
  test.after(async () => {
    await rm(configuredWorkspaceRoot, { recursive: true, force: true })
  })

  const source = workspacePath(fixtureRelativePath)
  const destination = join(configuredWorkspaceRoot, fixtureRelativePath)
  await mkdir(dirname(destination), { recursive: true })
  await cp(source, destination, { recursive: true })
  return configuredWorkspaceRoot
}

async function relativeFiles(root: string): Promise<string[]> {
  const entries = await readdir(root, { recursive: true, withFileTypes: true })
  return entries
    .filter((entry) => entry.isFile())
    .map((entry) => join(entry.parentPath, entry.name).slice(root.length + 1))
    .toSorted()
}
