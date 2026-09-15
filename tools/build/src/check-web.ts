import { readFile, readdir } from 'node:fs/promises'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspacePath } from './workspace.ts'

const scriptPath = fileURLToPath(import.meta.url)
const builtAssets = workspacePath('packages', 'web', 'dist')
const embeddedAssets = workspacePath('packages', 'cli', 'internal', 'server', 'static')
const developmentAssets = new Set(['mockServiceWorker.js'])

async function files(root: string, directory = root): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true })
  const nested = await Promise.all(
    entries.map(async (entry) => {
      const path = join(directory, entry.name)
      if (entry.isDirectory()) return files(root, path)
      return entry.isFile() ? [relative(root, path)] : []
    }),
  )
  return nested.flat()
}

async function availableFiles(root: string): Promise<string[]> {
  try {
    return await files(root)
  } catch {
    return []
  }
}

export async function webAssetDifferences(
  expectedRoot: string,
  actualRoot: string,
): Promise<string[]> {
  const [expected, actual] = await Promise.all([
    availableFiles(expectedRoot),
    availableFiles(actualRoot),
  ])
  const productionExpected = expected.filter((path) => !developmentAssets.has(path))
  const productionActual = actual.filter((path) => !developmentAssets.has(path))
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

async function main(): Promise<void> {
  const differences = await webAssetDifferences(builtAssets, embeddedAssets)
  if (differences.length === 0) return
  throw new Error(`embedded web assets differ; run pnpm run build:web\n${differences.join('\n')}`)
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === scriptPath) {
  await main()
}
