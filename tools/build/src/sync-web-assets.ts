import { cp, readFile, rm, stat } from 'node:fs/promises'
import { basename, dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { listFiles } from './files.ts'
import { embeddedAssets, workspacePath } from './workspace.ts'

const scriptPath = fileURLToPath(import.meta.url)
const source = workspacePath('packages', 'web', 'dist')
const target = workspacePath(embeddedAssets)
const developmentAssets = new Set(['mockServiceWorker.js'])

async function availableFiles(root: string): Promise<string[]> {
  try {
    return await listFiles(root)
  } catch {
    return []
  }
}

// Reports how embedded assets differ from the production subset of a web build.
export async function webAssetDifferences(
  expectedRoot: string,
  actualRoot: string,
): Promise<string[]> {
  const [expected, actual] = await Promise.all([
    availableFiles(expectedRoot),
    availableFiles(actualRoot),
  ])
  const productionExpected = expected.filter((path) => !developmentAssets.has(path))
  const productionActual = actual
  const paths = [...new Set([...productionExpected, ...productionActual])].toSorted()
  const differences = await Promise.all(
    paths.map(async (path) => {
      if (!productionExpected.includes(path)) {
        return `unexpected ${path}`
      }
      if (!productionActual.includes(path)) {
        return `missing ${path}`
      }
      const [expectedContents, actualContents] = await Promise.all([
        readFile(join(expectedRoot, path)),
        readFile(join(actualRoot, path)),
      ])
      return expectedContents.equals(actualContents) ? undefined : `changed ${path}`
    }),
  )
  return differences.filter((difference) => difference !== undefined)
}

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
    filter: (path) => !developmentAssets.has(relative(source, path)),
  })
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  await syncWebAssets()
}
