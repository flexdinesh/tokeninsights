import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { workspacePath, workspaceRoot } from './workspace.ts'

const concurrency = process.env.TOKENINSIGHTS_CHECK_CONCURRENCY ?? '2'
if (!/^[1-9]\d*$/.test(concurrency) || !Number.isSafeInteger(Number(concurrency))) {
  throw new Error('TOKENINSIGHTS_CHECK_CONCURRENCY must be a positive integer')
}
const directory = await mkdtemp(join(tmpdir(), 'tokeninsights-check-gate-'))
const binary = join(directory, 'check-gate')
const env = {
  ...process.env,
  GOMAXPROCS: process.env.GOMAXPROCS ?? concurrency,
  GOFLAGS: process.env.GOFLAGS ?? `-p=${concurrency}`,
}

async function run(command: string, args: string[], cwd: string): Promise<number> {
  const child = spawn(command, args, { cwd, env, stdio: ['inherit', 'inherit', 'inherit', 'pipe'] })
  const interrupt = () => child.kill('SIGINT')
  const terminate = () => child.kill('SIGTERM')
  process.on('SIGINT', interrupt)
  process.on('SIGTERM', terminate)
  try {
    return await new Promise<number>((resolve, reject) => {
      child.on('error', reject)
      child.on('exit', (code, signal) => resolve(code ?? (signal === 'SIGINT' ? 130 : 143)))
    })
  } finally {
    process.off('SIGINT', interrupt)
    process.off('SIGTERM', terminate)
    child.stdio[3]?.destroy()
  }
}

try {
  // Only this small development helper compiles before admission. All actual
  // verification and application compilation remain inside the gate.
  process.exitCode = await run(
    'go',
    ['build', '-o', binary, './tools/check-gate'],
    workspacePath('packages/cli'),
  )
  if (process.exitCode === 0) {
    process.exitCode = await run(
      binary,
      ['--watch-parent', 'mise', 'run', '--jobs', concurrency, 'check:push:stages'],
      workspaceRoot,
    )
  }
} finally {
  await rm(directory, { recursive: true, force: true })
}
