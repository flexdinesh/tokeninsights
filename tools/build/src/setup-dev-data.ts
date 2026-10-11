import { spawn } from 'node:child_process'
import { DatabaseSync } from 'node:sqlite'
import { copyFile, mkdir, readFile, readdir, symlink } from 'node:fs/promises'
import { basename, dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspaceRoot } from './workspace.ts'

const scriptPath = fileURLToPath(import.meta.url)
const fixtureRelativePath = join(
  'packages',
  'cli',
  'testdata',
  'conformance',
  'sync-first-basic',
  'source',
)

export interface SyncPaths {
  binaryPath: string
  collectorDBPath: string
  serverDBPath: string
  sourceDir: string
}

export type SyncFunction = (paths: SyncPaths) => Promise<void>

interface SetupDevDataOptions {
  workspaceRoot?: string
  sync?: SyncFunction
  prepare?: (binaryPath: string, collectorDBPath: string, serverDBPath: string) => Promise<void>
}

interface SetupDevDataResult {
  collectorDBPath: string
  serverDBPath: string
  outputDir: string
  sourceDir: string
}

export async function setupDevData({
  workspaceRoot: configuredWorkspaceRoot = workspaceRoot,
  sync = runSync,
  prepare = prepareFixture,
}: SetupDevDataOptions = {}): Promise<SetupDevDataResult> {
  const root = resolve(configuredWorkspaceRoot)
  const outputDir = join(root, '.tokeninsights-dev')
  assertControlledOutput(root, outputDir)

  const fixtureDir = join(root, fixtureRelativePath)
  const sourceDir = join(outputDir, 'source')
  const collectorDBPath = join(outputDir, 'collector.sqlite')
  const serverDBPath = join(outputDir, 'server.sqlite')

  await prepare(
    join(root, 'packages', 'cli', 'bin', 'tokeninsights'),
    collectorDBPath,
    serverDBPath,
  )
  await mkdir(sourceDir, { recursive: true })
  await copyJSONLFixtures(fixtureDir, sourceDir)
  await materializeOpenCode(fixtureDir, sourceDir)
  await setupSourceHome(outputDir, sourceDir)
  await sync({
    binaryPath: join(root, 'packages', 'cli', 'bin', 'tokeninsights'),
    collectorDBPath,
    serverDBPath,
    sourceDir,
  })

  return { collectorDBPath, serverDBPath, outputDir, sourceDir }
}

function assertControlledOutput(configuredWorkspaceRoot: string, outputDir: string): void {
  if (
    basename(outputDir) !== '.tokeninsights-dev' ||
    dirname(outputDir) !== configuredWorkspaceRoot
  ) {
    throw new Error(`refusing to recreate uncontrolled directory: ${outputDir}`)
  }
}

async function copyJSONLFixtures(fixtureDir: string, sourceDir: string): Promise<void> {
  const paths = await listFiles(fixtureDir)
  await Promise.all(
    paths
      .filter((path) => path.endsWith('.jsonl'))
      .map(async (path) => {
        const destination = join(sourceDir, relative(fixtureDir, path))
        await mkdir(dirname(destination), { recursive: true })
        await copyFile(path, destination)
      }),
  )
}

async function listFiles(dir: string): Promise<string[]> {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = await Promise.all(
    entries.map(async (entry) => {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) {
        return listFiles(path)
      }
      return entry.isFile() ? [path] : []
    }),
  )
  return files.flat()
}

async function materializeOpenCode(fixtureDir: string, sourceDir: string): Promise<void> {
  const sqlPath = join(fixtureDir, 'opencode', 'source.sql')
  const dbPath = join(sourceDir, 'opencode', 'opencode.db')
  await mkdir(dirname(dbPath), { recursive: true })
  const database = new DatabaseSync(dbPath)
  try {
    database.exec(await readFile(sqlPath, 'utf8'))
  } finally {
    database.close()
  }
}

async function runSync({
  binaryPath,
  collectorDBPath,
  serverDBPath,
  sourceDir,
}: SyncPaths): Promise<void> {
  await runCommand(binaryPath, [
    '__capture-dev-data',
    '--collector-db-path',
    collectorDBPath,
    '--server-db-path',
    serverDBPath,
    '--source-dir',
    sourceDir,
  ])
}

async function runCommand(binaryPath: string, args: string[]): Promise<void> {
  await new Promise<void>((resolveRun, rejectRun) => {
    const child = spawn(binaryPath, args, {
      stdio: 'inherit',
      env: { ...process.env, TOKENINSIGHTS_ACCESS_TOKEN: '' },
    })
    child.once('error', rejectRun)
    child.once('exit', (code, signal) => {
      if (code === 0) {
        resolveRun()
        return
      }
      rejectRun(
        new Error(
          signal === null
            ? `tokeninsights fixture command exited with code ${code}`
            : `tokeninsights fixture command exited with signal ${signal}`,
        ),
      )
    })
  })
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  const { collectorDBPath, serverDBPath } = await setupDevData()
  console.log(`development data ready: collector=${collectorDBPath} server=${serverDBPath}`)
}

async function prepareFixture(
  binaryPath: string,
  collectorDBPath: string,
  serverDBPath: string,
): Promise<void> {
  await new Promise<void>((resolveRun, rejectRun) => {
    const child = spawn(
      binaryPath,
      [
        '__prepare-dev-data',
        '--collector-db-path',
        collectorDBPath,
        '--server-db-path',
        serverDBPath,
      ],
      {
        stdio: 'inherit',
      },
    )
    child.once('error', rejectRun)
    child.once('exit', (code) => {
      if (code === 0) resolveRun()
      else rejectRun(new Error('fixture preparation failed; stop development viewer first'))
    })
  })
}
async function setupSourceHome(outputDir: string, sourceDir: string): Promise<void> {
  const links = [
    [join(outputDir, 'home', '.pi', 'agent', 'sessions'), join(sourceDir, 'pi')],
    [join(outputDir, 'home', '.codex', 'sessions'), join(sourceDir, 'codex')],
    [join(outputDir, 'home', '.claude', 'projects'), join(sourceDir, 'claude-code')],
    [join(outputDir, 'home', '.local', 'share', 'opencode'), join(sourceDir, 'opencode')],
  ]
  await Promise.all(
    links.map(async (pair) => {
      const link = pair[0]
      const target = pair[1]
      if (link === undefined || target === undefined) throw new Error('invalid fixture link')
      await mkdir(dirname(link), { recursive: true })
      await symlink(target, link, 'dir')
    }),
  )
}
