import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { listFiles } from './files.ts'
import { syncWebAssets, webAssetDifferences } from './sync-web-assets.ts'
import { embeddedAssets, workspacePath, workspaceRoot } from './workspace.ts'

// Lint, test, build and distribution tasks each require embedded assets. Rebuild
// only when web inputs change or the recorded build output drifted.
const scriptPath = fileURLToPath(import.meta.url)
const webRoot = workspacePath('packages', 'web')
const builtAssets = join(webRoot, 'dist')
const embeddedRoot = workspacePath(embeddedAssets)
const stampPath = join(webRoot, 'node_modules', '.cache', 'tokeninsights', 'web-build.txt')
const generatedEntries = new Set(['node_modules', 'dist', 'test-results', 'playwright-report'])
const sharedInputs = [
  'package.json',
  'pnpm-lock.yaml',
  'pnpm-workspace.yaml',
  'tools/build/src/build-web.ts',
  'tools/build/src/sync-web-assets.ts',
]

async function digest(root: string, paths: string[]): Promise<string> {
  const contents = await Promise.all(paths.map((path) => readFile(join(root, path))))
  const hash = createHash('sha256')
  paths.forEach((path, index) => {
    hash.update(`${path}\0`)
    hash.update(contents[index] ?? '')
    hash.update('\0')
  })
  return hash.digest('hex')
}

export async function directoryDigest(
  root: string,
  excluded: ReadonlySet<string> = new Set(),
): Promise<string> {
  return digest(root, await listFiles(root, excluded))
}

async function inputDigest(): Promise<string> {
  const web = await directoryDigest(webRoot, generatedEntries)
  const shared = await digest(workspaceRoot, sharedInputs)
  return `${web}:${shared}`
}

async function readOptional(path: string): Promise<string | undefined> {
  try {
    return await readFile(path, 'utf8')
  } catch {
    return undefined
  }
}

async function outputDigest(): Promise<string | undefined> {
  try {
    return await directoryDigest(builtAssets)
  } catch {
    return undefined
  }
}

async function main(): Promise<void> {
  const inputs = await inputDigest()
  const output = await outputDigest()
  const current =
    output !== undefined && (await readOptional(stampPath)) === `${inputs}\n${output}\n`
  if (current) {
    console.log('Web assets current; skipping web build')
  } else {
    const build = spawnSync('pnpm', ['--filter', '@tokeninsights/web', 'run', 'build'], {
      cwd: workspaceRoot,
      stdio: 'inherit',
    })
    if (build.error) throw build.error
    if (build.status !== 0) throw new Error('web build failed')
  }
  if (!current || (await webAssetDifferences(builtAssets, embeddedRoot)).length > 0) {
    await syncWebAssets()
  }
  if (!current) {
    const built = await outputDigest()
    if (built === undefined) throw new Error(`web build output missing: ${builtAssets}`)
    await mkdir(dirname(stampPath), { recursive: true })
    await writeFile(stampPath, `${inputs}\n${built}\n`)
  }
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  await main()
}
