import { spawn } from 'node:child_process'

export const syncFailure = 'TokenInsights sync failed; run tokeninsights sync for details.'
export const syncDeadlineMs = 60_000

export type RunnerOptions = { deadlineMs?: number; killGraceMs?: number }

// Plugins only trigger the finite Go collector. Event payloads never become
// process arguments, stdin, output, or diagnostics. Overlapping calls coalesce.
export class CollectorRunner {
  private active: Promise<void> | undefined
  private terminate: (() => void) | undefined
  private closed = false
  private options: RunnerOptions

  constructor(options: RunnerOptions = {}) {
    this.options = options
  }

  run(): Promise<void> {
    if (this.closed) return Promise.reject(new Error(syncFailure))
    if (this.active !== undefined) return this.active
    const active = this.launch().finally(() => {
      this.active = undefined
      this.terminate = undefined
    })
    this.active = active
    return active
  }

  async close(): Promise<void> {
    this.closed = true
    this.terminate?.()
    await this.active?.catch(() => {})
  }

  private launch(): Promise<void> {
    return new Promise((resolve, reject) => {
      let stopped = false
      let escalation: ReturnType<typeof setTimeout> | undefined
      const child = spawn(process.env.TOKENINSIGHTS_BINARY ?? 'tokeninsights', ['sync'], {
        detached: process.platform !== 'win32',
        stdio: 'ignore',
        shell: false,
      })
      const kill = (signal: NodeJS.Signals): void => {
        if (child.pid === undefined) return
        try {
          if (process.platform === 'win32') child.kill(signal)
          else process.kill(-child.pid, signal)
        } catch {
          // A concurrent child exit already releases this process group.
        }
      }
      const stop = (): void => {
        if (stopped) return
        stopped = true
        kill('SIGTERM')
        escalation = setTimeout(() => kill('SIGKILL'), this.options.killGraceMs ?? 1_000)
        escalation.unref()
      }
      this.terminate = stop
      const deadline = setTimeout(stop, this.options.deadlineMs ?? syncDeadlineMs)
      deadline.unref()
      const finish = (success: boolean): void => {
        clearTimeout(deadline)
        clearTimeout(escalation)
        if (stopped) kill('SIGKILL')
        if (success && !stopped) resolve()
        else reject(new Error(syncFailure))
      }
      child.once('error', () => finish(false))
      child.once('exit', (code) => finish(code === 0))
    })
  }
}
