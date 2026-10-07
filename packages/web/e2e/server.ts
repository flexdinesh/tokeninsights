import { spawn } from 'node:child_process'
import type { Server } from 'node:http'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import {
  adminRequest,
  hostedBackend,
  hostedFixturePointer,
  hostedOrigin,
  seedHostedUser,
  startHostedProxy,
  tokenSchema,
  userSchema,
} from './hosted-fixture.ts'

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

// Usable counters with no native message identity remain estimated only.
await writeFile(
  join(localHome, '.pi/agent/sessions/estimated.jsonl'),
  [
    JSON.stringify({ type: 'session', id: 'estimated-session' }),
    JSON.stringify({
      type: 'message',
      timestamp: now.toISOString(),
      message: {
        role: 'assistant',
        provider: 'openai',
        model: 'estimated-model',
        usage: { input: 100, output: 20 },
      },
    }),
  ].join('\n'),
)

function startServer(home: string, port: string) {
  return spawn(
    resolve('../cli/bin/tokeninsights-server'),
    ['--listen', '0.0.0.0:' + port, '--server-db-path', join(home, 'server.duckdb')],
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

// Seed through acceptance; wait for async projection before exposing tests.
const fixtureServer = startServer(localHome, '18766')
const fixtureDeadline = Date.now() + 10000
try {
  for (;;) {
    if (fixtureServer.exitCode !== null || fixtureServer.signalCode !== null) {
      throw new Error('fixture server exited before readiness')
    }
    try {
      const response = await fetch('http://127.0.0.1:18766/api/v2/instance')
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
        join(localHome, 'server.duckdb'),
        '--server-url',
        'http://127.0.0.1:18766',
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
  const processingDeadline = Date.now() + 30000
  for (;;) {
    const response = await fetch('http://127.0.0.1:18766/api/v2/usage?period=all&tab=models')
    const value: unknown = await response.json()
    if (
      response.ok &&
      typeof value === 'object' &&
      value !== null &&
      'pending' in value &&
      value.pending === 0 &&
      'summary' in value &&
      typeof value.summary === 'object' &&
      value.summary !== null &&
      'total' in value.summary &&
      value.summary.total === 344000
    )
      break
    if (Date.now() >= processingDeadline) throw new Error('fixture processing timed out')
    await new Promise<void>((resolveWait) => setTimeout(resolveWait, 50))
  }
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
const hostedHome = join(localHome, 'hosted')
await mkdir(hostedHome)
const hostedSocket = join(hostedHome, 'admin.sock')
const hostedServer = spawn(
  resolve('../cli/bin/tokeninsights-server'),
  [
    '--kind',
    'hosted',
    '--listen',
    '127.0.0.1:18768',
    '--server-db-path',
    join(hostedHome, 'server.duckdb'),
    '--public-url',
    hostedOrigin,
    '--admin-socket',
    hostedSocket,
  ],
  {
    stdio: 'inherit',
    env: {
      ...process.env,
      HOME: hostedHome,
      XDG_DATA_HOME: join(hostedHome, '.local/share'),
      XDG_CONFIG_HOME: join(hostedHome, '.config'),
    },
  },
)
const children = [startServer(localHome, '18765'), hostedServer]

let stopping = false
let proxy: Server | undefined

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
  const activeProxy = proxy
  if (activeProxy)
    await new Promise<void>((resolveClose) => activeProxy.close(() => resolveClose()))
  await rm(localHome, { recursive: true, force: true })
  await rm(hostedFixturePointer, { force: true })
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

try {
  const hostedDeadline = Date.now() + 10000
  for (;;) {
    if (hostedServer.exitCode !== null || hostedServer.signalCode !== null)
      throw new Error('Hosted fixture exited before readiness')
    try {
      if ((await fetch(hostedBackend + '/readyz')).ok) break
    } catch {
      /* Bounded startup. */
    }
    if (Date.now() >= hostedDeadline) throw new Error('Hosted fixture readiness timed out')
    await new Promise<void>((resolveWait) => setTimeout(resolveWait, 50))
  }
  for (const { name, input } of [
    { name: 'alice', input: 100 },
    { name: 'bob', input: 400 },
  ]) {
    const user = await adminRequest(
      hostedSocket,
      { operation: 'create-user', displayName: name },
      userSchema,
    )
    const token = await adminRequest(
      hostedSocket,
      { operation: 'create-token', userId: user.userId, permissions: ['read', 'ingest'] },
      tokenSchema,
    )
    await seedHostedUser(token.token, input)
    await writeFile(join(hostedHome, name + '.json'), JSON.stringify({ ...user, ...token }))
  }
  await writeFile(join(hostedHome, 'socket.txt'), hostedSocket)
  // Fixture files are available only to the Node test process through this local pointer.
  await writeFile(hostedFixturePointer, hostedHome, { mode: 0o600 })
  proxy = await startHostedProxy()
} catch (error) {
  await stop(1)
  throw error
}
