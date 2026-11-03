import { spawn } from 'node:child_process'
import { localBackendDescriptorSchema } from '@tokeninsights/contracts/local'
import type { LocalBackendDescriptor } from '@tokeninsights/contracts/local'
import { localTarget } from './links.ts'

const startupTimeoutMs = 30_000
const descriptorMaxBytes = 16_384

export interface BackendConnection {
  descriptor: LocalBackendDescriptor
  close(): void
}

// A URL handoff identifies an existing owner; it must never select defaults.
export function backendArguments(endpoint?: string): string[] {
  if (endpoint === undefined) return ['connect', '--json']
  if (!localTarget(endpoint)) throw new Error('Expected a local dashboard address.')
  return ['connect', '--json', '--url', new URL(endpoint).origin]
}

// The connector owns the viewer lease. Never detach it or close stdin while the
// window lives. Go resolves config/storage and starts or discovers the owner.
export async function connectBackend(
  executable: string,
  onExit: () => void,
  args: string[] = ['connect', '--json'],
): Promise<BackendConnection> {
  const child = spawn(executable, args, { stdio: ['pipe', 'pipe', 'inherit'], shell: false })
  let closing = false
  let ready = false
  const close = () => {
    if (closing) return
    closing = true
    child.stdin.end()
  }
  const descriptor = await new Promise<LocalBackendDescriptor>((resolve, reject) => {
    let output = ''
    const fail = (error: Error) => {
      clearTimeout(timeout)
      close()
      child.kill()
      reject(error)
    }
    const timeout = setTimeout(
      () => fail(new Error('Local backend startup timed out.')),
      startupTimeoutMs,
    )
    child.once('error', fail)
    child.stdin.on('error', (error) => {
      if (!ready) fail(error)
    })
    child.once('exit', (code, signal) => {
      clearTimeout(timeout)
      if (!ready)
        reject(new Error(`Local backend connector exited before readiness (${signal ?? code}).`))
      else if (!closing) onExit()
    })
    child.stdout.setEncoding('utf8')
    child.stdout.on('data', (chunk: string) => {
      if (ready || closing) return
      output += chunk
      if (Buffer.byteLength(output) > descriptorMaxBytes) {
        fail(new Error('Local backend descriptor exceeds size limit.'))
        return
      }
      const newline = output.indexOf('\n')
      if (newline < 0) return
      try {
        const value: unknown = JSON.parse(output.slice(0, newline))
        const result = localBackendDescriptorSchema.parse(value)
        ready = true
        clearTimeout(timeout)
        resolve(result)
      } catch {
        fail(new Error('Local backend returned an incompatible connection descriptor.'))
      }
    })
  })
  return { descriptor, close }
}
