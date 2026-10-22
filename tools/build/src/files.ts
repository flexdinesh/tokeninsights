import { readdir } from 'node:fs/promises'
import { join } from 'node:path'

// Lists regular files under root, relative to it and sorted. Symbolic links are
// never followed: recursive readdir follows them from Node 26.12. Entries whose
// name is in skip are ignored at any depth.
export async function listFiles(
  root: string,
  skip: ReadonlySet<string> = new Set(),
): Promise<string[]> {
  async function visit(directory: string): Promise<string[]> {
    const entries = await readdir(join(root, directory), { withFileTypes: true })
    const nested = await Promise.all(
      entries
        .filter((entry) => !skip.has(entry.name))
        .map((entry) => {
          const path = join(directory, entry.name)
          if (entry.isDirectory()) return visit(path)
          return Promise.resolve(entry.isFile() ? [path] : [])
        }),
    )
    return nested.flat()
  }
  return (await visit('')).toSorted()
}
