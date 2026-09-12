import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const moduleDir = dirname(fileURLToPath(import.meta.url))

export const workspaceRoot = resolve(moduleDir, '../../..')

export function workspacePath(...segments: string[]): string {
  return join(workspaceRoot, ...segments)
}
