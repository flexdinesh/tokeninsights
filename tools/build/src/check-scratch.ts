import { execFileSync } from 'node:child_process'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspaceRoot } from './workspace.ts'

export function checkScratchFiles(root: string): void {
  const paths = execFileSync('git', ['ls-files', '--cached', '--full-name', '-z'], {
    cwd: root,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).split('\0')
  const scratch = paths.filter((path) => path.split('/').slice(0, -1).includes('.scratch'))
  if (scratch.length === 0) return
  throw new Error(
    `Scratch files must remain local. Untrack with git rm --cached:\n${scratch.join('\n')}`,
  )
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  checkScratchFiles(workspaceRoot)
}
