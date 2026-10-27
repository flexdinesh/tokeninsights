import { spawn } from 'node:child_process'
import type { SpawnOptions } from 'node:child_process'
import { stripVTControlCharacters } from 'node:util'

const startupTimeoutMs = 30_000
const shutdownTimeoutMs = 5_000
const maximumLineLength = 16_384

export function parsePort(value: string): number {
  if (!/^\d+$/.test(value) || Number(value) > 65535) {
    throw new Error(`Invalid port: ${value}; expected 0..65535`)
  }
  return Number(value)
}

export function parseDevOptions(args: string[], mode: 'server' | 'combined') {
  let apiPort = 0
  let webPort = 0
  for (let index = 0; index < args.length; index += 1) {
    const flag = args[index]
    const value = args[index + 1]
    if (
      value === undefined ||
      (flag !== '--api-port' && !(mode === 'combined' && flag === '--web-port'))
    ) {
      throw new Error(`Expected --api-port N${mode === 'combined' ? ' or --web-port N' : ''}`)
    }
    if (flag === '--api-port') apiPort = parsePort(value)
    else webPort = parsePort(value)
    index += 1
  }
  return { apiPort, webPort }
}

export function startDevChild(
  command: string,
  args: string[],
  options: SpawnOptions,
  readyPattern: RegExp,
  timeoutMs = startupTimeoutMs,
  write: (chunk: string) => void = (chunk) => process.stdout.write(chunk),
) {
  const child = spawn(command, args, { ...options, stdio: ['ignore', 'pipe', 'pipe'] })
  const exited = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>((resolve) => {
    child.once('exit', (code, signal) => resolve({ code, signal }))
    child.once('error', () => resolve({ code: 1, signal: null }))
  })
  const ready = new Promise<string>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`${command}: startup timed out`)), timeoutMs)
    let pending = ''
    child.stdout?.on('data', (chunk: Buffer) => {
      const text = chunk.toString()
      write(text)
      pending += stripVTControlCharacters(text)
      const lines = pending.split('\n')
      pending = lines.pop() ?? ''
      for (const line of lines) {
        const match = readyPattern.exec(line)
        if (match?.[1]) {
          clearTimeout(timer)
          resolve(match[1])
        }
      }
      pending = pending.slice(-maximumLineLength)
    })
    child.stderr?.on('data', (chunk: Buffer) => write(chunk.toString()))
    child.once('error', (error) => {
      clearTimeout(timer)
      reject(error)
    })
    void exited.then(({ code, signal }) => {
      clearTimeout(timer)
      reject(new Error(`${command}: exited before readiness (${signal ?? code})`))
    })
  })
  // Cancellation can race startup; callers still receive the original rejection.
  void ready.catch(() => {})
  let stopping: Promise<void> | undefined
  function stop(graceMs = shutdownTimeoutMs): Promise<void> {
    stopping ??= (async () => {
      if (child.exitCode !== null || child.signalCode !== null || child.pid === undefined) return
      child.kill('SIGTERM')
      const timer = setTimeout(() => child.kill('SIGKILL'), graceMs)
      try {
        await exited
      } finally {
        clearTimeout(timer)
      }
    })()
    return stopping
  }
  return { ready, exited, stop }
}

export type DevChild = ReturnType<typeof startDevChild>

export async function runDevSession(
  startAPI: () => DevChild,
  startWeb: ((apiURL: string) => DevChild) | undefined,
  signal: AbortSignal,
  report: (message: string) => void = console.log,
) {
  const children: DevChild[] = []
  const { promise: aborted, reject } = Promise.withResolvers<never>()
  const onAbort = () => reject(new Error('Development stopped'))
  signal.addEventListener('abort', onAbort, { once: true })
  try {
    signal.throwIfAborted()
    const api = startAPI()
    children.push(api)
    const apiURL = await Promise.race([api.ready, aborted])
    report(`API: ${apiURL}`)
    const apiExit = api.exited.then(({ code, signal: exitSignal }) => {
      throw new Error(`API exited (${exitSignal ?? code})`)
    })
    // Observe exits during web startup and throughout standalone sessions.
    void apiExit.catch(() => {})
    if (startWeb) {
      const web = startWeb(apiURL)
      children.push(web)
      const webURL = await Promise.race([web.ready, apiExit, aborted])
      report(`Web: ${webURL}`)
    }
    await Promise.race([
      aborted,
      ...children.map(async (child) => {
        const result = await child.exited
        throw new Error(`Development child exited (${result.signal ?? result.code})`)
      }),
    ])
  } finally {
    signal.removeEventListener('abort', onAbort)
    await Promise.all(children.map((child) => child.stop()))
  }
}
