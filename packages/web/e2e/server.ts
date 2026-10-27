import { spawn, type ChildProcess } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import type { Server } from 'node:http'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { z } from 'zod'
import { CollectorProgressResponse, StatusResponseV2 } from '../src/generated/api.ts'
import {
  adminRequest,
  seedHostedUser,
  startHostedProxy,
  tokenSchema,
  userSchema,
} from './hosted-fixture.ts'
import { metadataSchema, runDirectory } from './fixture.ts'

const readinessTimeout = 30_000
const shutdownTimeout = 5_000
const pollInterval = 50
const requestTimeout = 1_000
const diagnosticLimit = 64 * 1024
const delay = (milliseconds: number) => new Promise<void>((done) => setTimeout(done, milliseconds))

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

function localEnvironment(home: string): NodeJS.ProcessEnv {
  return {
    PATH: process.env.PATH,
    TZ: 'UTC',
    NO_COLOR: '1',
    HOME: home,
    TOKENINSIGHTS_SERVER_URL: '',
    TOKENINSIGHTS_ACCESS_TOKEN: '',
    XDG_DATA_HOME: join(home, '.local/share'),
    XDG_CONFIG_HOME: join(home, '.config'),
    XDG_STATE_HOME: join(home, '.local/state'),
    XDG_RUNTIME_DIR: '',
    CODEX_HOME: join(home, '.codex'),
    CLAUDE_CONFIG_DIR: join(home, '.claude'),
  }
}

function startChild(executable: string, args: string[], environment: NodeJS.ProcessEnv) {
  const child = spawn(resolve('../cli/bin', executable), args, {
    stdio: ['ignore', 'pipe', 'pipe'],
    env: environment,
  })
  let output = ''
  let failure: Error | undefined
  const record = (chunk: Buffer) => {
    output = (output + chunk.toString()).slice(-diagnosticLimit)
  }
  child.stdout.on('data', record)
  child.stderr.on('data', record)
  child.on('error', (error) => {
    failure = error
  })
  return {
    child,
    output: () => output,
    assertRunning() {
      if (failure) throw failure
      if (child.exitCode !== null || child.signalCode !== null)
        throw new Error(`Fixture exited before readiness: ${output}`)
    },
  }
}

async function stopChild(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null || child.pid === undefined) return
  await new Promise<void>((done, reject) => {
    const finish = () => {
      clearTimeout(kill)
      clearTimeout(deadline)
      done()
    }
    const kill = setTimeout(() => child.kill('SIGKILL'), shutdownTimeout)
    const deadline = setTimeout(() => {
      child.off('exit', finish)
      reject(new Error('Fixture child did not exit after SIGKILL'))
    }, shutdownTimeout * 2)
    child.once('exit', finish)
    child.kill('SIGTERM')
  })
}

async function waitUntil(check: () => Promise<boolean>, label: string) {
  const deadline = Date.now() + readinessTimeout
  while (!(await check())) {
    if (Date.now() >= deadline) throw new Error(`${label} timed out`)
    await delay(pollInterval)
  }
}

async function boundURL(process: ReturnType<typeof startChild>, signal: AbortSignal) {
  let address = ''
  await waitUntil(async () => {
    signal.throwIfAborted()
    process.assertRunning()
    address = process.output().match(/http:\/\/127\.0\.0\.1:\d+/)?.[0] ?? ''
    return address !== ''
  }, 'Bound URL discovery')
  return address
}

export default async function setup() {
  const directory = runDirectory()
  await mkdir(directory, { recursive: true, mode: 0o700 })
  // Keep Unix sockets short even when the checkout has a long absolute path.
  const localHome = await mkdtemp(join(tmpdir(), 'ti-e2e-'))
  const hostedHome = join(localHome, 'hosted')
  const hostedPassword = randomUUID()
  const children: ReturnType<typeof startChild>[] = []
  const cancellation = new AbortController()
  let ready = false
  let proxy: Server | undefined
  let cleanup: Promise<void> | undefined
  const stop = () => {
    cleanup ??= (async () => {
      const activeProxy = proxy
      const closing = activeProxy
        ? new Promise<void>((done, reject) => {
            activeProxy.close((error) => (error ? reject(error) : done()))
            activeProxy.closeAllConnections()
          })
        : Promise.resolve()
      const stopped = await Promise.allSettled([
        closing,
        ...children.map(({ child }) => stopChild(child)),
      ])
      const finalized = await Promise.allSettled([
        ...children.map((child, index) =>
          writeFile(join(directory, `server-${index}.log`), child.output(), { mode: 0o600 }),
        ),
        rm(join(directory, 'fixture.json'), { force: true }),
        rm(localHome, { recursive: true, force: true }),
      ])
      process.off('SIGINT', onSignal)
      process.off('SIGTERM', onSignal)
      for (const result of [...stopped, ...finalized])
        if (result.status === 'rejected') throw result.reason
    })()
    return cleanup
  }
  const onSignal = () => {
    cancellation.abort(new Error('Browser fixture setup interrupted'))
    if (ready)
      void stop().catch((error: unknown) => {
        console.error(error)
        process.exitCode = 1
      })
  }
  process.once('SIGINT', onSignal)
  process.once('SIGTERM', onSignal)

  try {
    const now = new Date()
    for (let index = 0; index < 80; index++) {
      cancellation.signal.throwIfAborted()
      const model = index % 2 === 0 ? 'model-a' : 'model-b'
      const provider = index % 2 === 0 ? 'openai' : 'anthropic'
      const recordedAt = new Date(now)
      if (index >= 60) recordedAt.setFullYear(now.getFullYear() - 1)
      const cwd = index % 5 === 4 ? undefined : join(localHome, 'workspace', `project-${index % 4}`)
      await writeSession(
        localHome,
        index,
        'web-session',
        model,
        provider,
        4300,
        3000,
        recordedAt,
        cwd,
      )
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

    await mkdir(hostedHome, { mode: 0o700 })
    const hostedSocket = join(hostedHome, 'admin.sock')
    let hostedBackend = ''
    proxy = await startHostedProxy(() => hostedBackend)
    cancellation.signal.throwIfAborted()
    const address = proxy.address()
    if (!address || typeof address === 'string') throw new Error('Expected HTTPS TCP listener')
    const hostedOrigin = `https://127.0.0.1:${address.port}`
    const local = startChild(
      'tokeninsights',
      [
        'web',
        '--open=false',
        '--host',
        '127.0.0.1',
        '--port',
        '0',
        '--collector-db-path',
        join(localHome, 'collector.sqlite'),
        '--server-db-path',
        join(localHome, 'server.sqlite'),
      ],
      localEnvironment(localHome),
    )
    children.push(local)
    const hosted = startChild(
      'tokeninsights-server',
      [
        '--listen',
        '127.0.0.1:0',
        '--server-db-path',
        join(hostedHome, 'server.sqlite'),
        '--public-url',
        hostedOrigin,
        '--admin-socket',
        hostedSocket,
        '--signup',
        '--trusted-proxies',
        '127.0.0.1/32',
      ],
      localEnvironment(hostedHome),
    )
    children.push(hosted)
    const localOrigin = await boundURL(local, cancellation.signal)
    hostedBackend = await boundURL(hosted, cancellation.signal)
    await waitUntil(async () => {
      local.assertRunning()
      cancellation.signal.throwIfAborted()
      const progress = CollectorProgressResponse.parse(
        await (
          await fetch(localOrigin + '/api/v2/collector-progress', {
            signal: AbortSignal.timeout(requestTimeout),
          })
        ).json(),
      )
      if (progress.attempts.some((attempt) => ['failed', 'interrupted'].includes(attempt.stage)))
        throw new Error('Local capture failed')
      if (!progress.attempts.some((attempt) => attempt.stage === 'accepted')) return false
      const status = StatusResponseV2.parse(
        await (
          await fetch(localOrigin + '/api/v2/status', {
            signal: AbortSignal.timeout(requestTimeout),
          })
        ).json(),
      )
      return status.pending === 0 && status.generation === status.targetGeneration
    }, 'Local capture')
    await waitUntil(async () => {
      hosted.assertRunning()
      cancellation.signal.throwIfAborted()
      return (
        await fetch(hostedBackend + '/readyz', { signal: AbortSignal.timeout(requestTimeout) })
      ).ok
    }, 'Hosted readiness')
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
        {
          operation: 'create-token',
          userId: user.userId,
          permissions: ['read', 'ingest'],
        },
        tokenSchema,
      )
      await adminRequest(
        hostedSocket,
        {
          operation: 'reset-password',
          userId: user.userId,
          password: hostedPassword,
        },
        z.object({}),
      )
      await seedHostedUser(hostedBackend, token.token, input, cancellation.signal)
      await writeFile(join(hostedHome, name + '.json'), JSON.stringify({ ...user, ...token }), {
        mode: 0o600,
      })
    }
    const metadata = metadataSchema.parse({
      localOrigin,
      hostedOrigin,
      hostedBackend,
      hostedHome,
      hostedPassword,
    })
    cancellation.signal.throwIfAborted()
    await writeFile(join(directory, 'fixture.json'), JSON.stringify(metadata), { mode: 0o600 })
    cancellation.signal.throwIfAborted()
    ready = true
    return stop
  } catch (error) {
    await stop()
    throw error
  }
}
