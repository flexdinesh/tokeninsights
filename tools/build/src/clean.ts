import { rm } from 'node:fs/promises'

import { workspacePath } from './workspace.ts'

const paths = [
  workspacePath('packages', 'cli', 'tokeninsights-cli'),
  workspacePath('packages', 'cli', 'tokeninsights'),
  workspacePath('packages', 'cli', 'bin'),
]

await Promise.all(paths.map((path) => rm(path, { recursive: true, force: true })))
