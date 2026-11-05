import { spawn } from 'node:child_process'
import { localBackendDescriptorSchema } from '@tokeninsights/contracts/local'
import type { LocalBackendDescriptor } from '@tokeninsights/contracts/local'
import { launchURL } from './links.ts'

const startupTimeoutMs = 30_000
const descriptorMaxBytes = 16_384

export interface BackendConnection {
  descriptor: LocalBackendDescriptor
  close(): void
  acknowledge(): void
}

// argv contains only a random rendezvous ID. Connection secrets stay native.
export function backendArguments(link?: string): string[] {
  if (link === undefined) return ['connect', '--json']
  const valid = launchURL(link)
  if (valid === undefined) throw new Error('Invalid desktop launch link; run tokeninsights again.')
  return ['connect', '--json', '--launch', valid]
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
      else if (!closing) onExit()
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
  let acknowledged = false
  const acknowledge = () => {
    if (acknowledged || closing) return
    acknowledged = true
    child.stdin.write('ready\n')
  }
  return { descriptor, close, acknowledge }
}
