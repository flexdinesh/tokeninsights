import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { homedir } from 'node:os'
import { workspaceRoot } from './workspace.ts'

const root = join(workspaceRoot, '.tokeninsights-dev')
const home = join(root, 'home')
const child = spawn(
  join(workspaceRoot, 'packages/cli/bin/tokeninsights'),
  ['service', 'run', '--host', '127.0.0.1', '--db-path', join(root, 'tokeninsights.sqlite')],
  {
    stdio: 'inherit',
    env: {
      ...process.env,
      HOME: home,
      XDG_DATA_HOME: join(home, '.local/share'),
      XDG_CONFIG_HOME: process.env.XDG_CONFIG_HOME || join(homedir(), '.config'),
      XDG_STATE_HOME: process.env.XDG_STATE_HOME || join(homedir(), '.local/state'),
      XDG_RUNTIME_DIR: process.env.XDG_RUNTIME_DIR || '',
      CODEX_HOME: join(home, '.codex'),
      CLAUDE_CONFIG_DIR: join(home, '.claude'),
    },
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
