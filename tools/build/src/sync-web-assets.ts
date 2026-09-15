import { cp, rm, stat } from 'node:fs/promises'
import { basename, dirname, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspacePath } from './workspace.ts'

const scriptPath = fileURLToPath(import.meta.url)
const source = workspacePath('packages', 'web', 'dist')
const target = workspacePath('packages', 'cli', 'internal', 'server', 'static')

export async function syncWebAssets(): Promise<void> {
  const sourceInfo = await stat(source)
  if (!sourceInfo.isDirectory()) throw new Error(`web build output missing: ${source}`)
  if (
    basename(target) !== 'static' ||
    dirname(target) !== workspacePath('packages', 'cli', 'internal', 'server')
  ) {
    throw new Error(`invalid embedded asset target: ${target}`)
  }

  await rm(target, { force: true, recursive: true })
  await cp(source, target, {
    recursive: true,
    filter: (path) => relative(source, path) !== 'mockServiceWorker.js',
  })
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  await syncWebAssets()
}
