import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { workspaceRoot } from './workspace.ts'

const root = join(workspaceRoot, '.tokeninsights-dev')
const child = spawn(
  join(workspaceRoot, 'packages/cli/bin/tokeninsights'),
  [
    'service',
    'run',
    '--host',
    '127.0.0.1',
    '--port',
    '8765',
    '--server-db-path',
    join(root, 'server.sqlite'),
    '--token',
    '',
  ],
  {
    stdio: 'inherit',
  },
)
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill('SIGTERM'))
child.once('error', (error) => {
  console.error(error)
  process.exitCode = 1
})
child.once('exit', (code) => {
  process.exitCode = code ?? 1
})
