import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fixtureToken } from './config.ts'

const tempRoot = process.env.TOKENINSIGHTS_TEST_TMP ?? tmpdir()
const localHome = await mkdtemp(join(tempRoot, 'ti-web-'))
const now = new Date()

async function writeSession(
  home: string,
  index: number,
  prefix: string,
  model: string,
  provider: string,
  totalTokens: number,
  cacheRead: number,
  recordedAt: Date,
  cwd?: string,
) {
  const sessions = join(home, '.pi/agent/sessions')
  await mkdir(sessions, { recursive: true })
  const session = `${prefix}-${String(index).padStart(3, '0')}`
  const input = totalTokens - cacheRead - 300
  const records = [
    { type: 'session', version: 1, id: session, timestamp: recordedAt.toISOString(), cwd },
    {
      type: 'message',
      id: 'turn-1',
      timestamp: recordedAt.toISOString(),
      message: {
        role: 'assistant',
        model,
        provider,
        timestamp: recordedAt.getTime(),
        usage: { input, output: 200, cacheRead, cacheWrite: 100, totalTokens },
      },
    },
  ]
  await writeFile(
    join(sessions, `${session}.jsonl`),
    records.map((record) => JSON.stringify(record)).join('\n'),
  )
}

for (let index = 0; index < 80; index++) {
  const model = index % 2 === 0 ? 'model-a' : 'model-b'
  const provider = index % 2 === 0 ? 'openai' : 'anthropic'
  const recordedAt = new Date(now)
  if (index >= 60) recordedAt.setFullYear(now.getFullYear() - 1)
  const cwd = index % 5 === 4 ? undefined : join(localHome, 'workspace', `project-${index % 4}`)
  await writeSession(localHome, index, 'web-session', model, provider, 4300, 3000, recordedAt, cwd)
}

function startServer(home: string, port: string) {
  return spawn(
    resolve('../cli/bin/tokeninsights'),
    [
      'service',
      'run',
      '--host',
      '0.0.0.0',
      '--port',
      port,
      '--token',
      fixtureToken,
      '--server-db-path',
      join(home, 'server.sqlite'),
    ],
    {
      stdio: 'inherit',
      env: {
        ...process.env,
        HOME: home,
        XDG_DATA_HOME: join(home, '.local/share'),
        XDG_CONFIG_HOME: join(home, '.config'),
        XDG_STATE_HOME: join(home, '.local/state'),
        XDG_RUNTIME_DIR: '',
        CODEX_HOME: join(home, '.codex'),
        CLAUDE_CONFIG_DIR: join(home, '.claude'),
      },
    },
  )
}

// Seed through real canonical ingestion before exposing the Playwright endpoint.
const fixtureServer = startServer(localHome, '18766')
const fixtureDeadline = Date.now() + 10000
try {
  for (;;) {
    if (fixtureServer.exitCode !== null || fixtureServer.signalCode !== null) {
      throw new Error('fixture server exited before readiness')
    }
    try {
      const response = await fetch('http://127.0.0.1:18766/api/v1/instance', {
        headers: { Authorization: `Bearer ${fixtureToken}` },
      })
      if (response.ok) break
    } catch {
      // Startup is bounded; no host harness files are used.
    }
    if (Date.now() >= fixtureDeadline) throw new Error('fixture server readiness timed out')
    await new Promise<void>((resolveWait) => setTimeout(resolveWait, 50))
  }
  await new Promise<void>((resolveRun, rejectRun) => {
    const child = spawn(
      resolve('../cli/bin/tokeninsights'),
      [
        'sync',
        '--all',
        '--collector-db-path',
        join(localHome, 'collector.sqlite'),
        '--server-db-path',
        join(localHome, 'server.sqlite'),
        '--server-url',
        'http://127.0.0.1:18766',
        '--token',
        fixtureToken,
      ],
      {
        stdio: 'inherit',
        env: {
          ...process.env,
          HOME: localHome,
          XDG_DATA_HOME: join(localHome, '.local/share'),
          CODEX_HOME: join(localHome, '.codex'),
          CLAUDE_CONFIG_DIR: join(localHome, '.claude'),
          XDG_CONFIG_HOME: join(localHome, '.config'),
          XDG_STATE_HOME: join(localHome, '.local/state'),
          XDG_RUNTIME_DIR: '',
        },
      },
    )
    child.once('error', rejectRun)
    child.once('exit', (code) => {
      if (code === 0) resolveRun()
      else rejectRun(new Error('fixture sync failed'))
    })
  })
} finally {
  await new Promise<void>((resolveStop) => {
    if (fixtureServer.exitCode !== null || fixtureServer.signalCode !== null) {
      resolveStop()
      return
    }
    fixtureServer.once('exit', () => resolveStop())
    fixtureServer.kill('SIGTERM')
  })
}
const children = [startServer(localHome, '18765')]
let stopping = false

async function stop(code: number) {
  if (stopping) return
  stopping = true
  await Promise.all(
    children.map(
      (child) =>
        new Promise<void>((resolveStop) => {
          if (child.exitCode !== null || child.signalCode !== null) {
            resolveStop()
            return
          }
          child.once('exit', () => resolveStop())
          child.kill('SIGTERM')
        }),
    ),
  )
  await rm(localHome, { recursive: true, force: true })
  process.exitCode = code
}

process.on('SIGINT', () => void stop(0))
process.on('SIGTERM', () => void stop(0))
for (const child of children) {
  child.on('exit', (code) => {
    if (!stopping) void stop(code ?? 1)
  })
  child.on('error', (error) => {
    console.error(error)
    void stop(1)
  })
}
