import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const moduleDir = dirname(fileURLToPath(import.meta.url))

export const workspaceRoot = resolve(moduleDir, '../../..')

export function workspacePath(...segments: string[]): string {
  return join(workspaceRoot, ...segments)
}

// Workspace-relative, slash-separated directory that Go embeds. Generated on demand;
// ignored on main and committed only on installable refs.
export const embeddedAssets = 'packages/cli/internal/server/static'
