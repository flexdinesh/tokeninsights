import { spawn } from 'node:child_process'

export const syncFailure = 'TokenInsights sync failed; run tokeninsights sync for details.'
export const syncDeadlineMs = 60_000

// Plugins only trigger the finite Go collector. Event payloads never become
// process arguments, stdin, output, or diagnostics. Overlapping calls coalesce.
export class CollectorRunner {
  active
  terminate
  closed = false
  followup = false
  options

  constructor(options) {
    this.options = options
  }

  run() {
    if (this.closed) return Promise.reject(new Error(syncFailure))
    if (this.active !== undefined) {
      this.followup = true
      return this.active
    }
    const active = this.drain().finally(() => {
      this.active = undefined
      this.terminate = undefined
    })
    this.active = active
    return active
  }

  async close() {
    this.closed = true
    this.terminate?.()
    await this.active?.catch(() => {})
  }

  async drain() {
    this.followup = false
    await this.launch()
    if (this.followup && !this.closed) await this.drain()
  }

  launch() {
    return new Promise((resolve, reject) => {
      let stopped = false
      let escalation
      const child = spawn(
        process.env.TOKENINSIGHTS_BINARY ?? 'tokeninsights',
        ['sync', '--wait', '--harness', this.options.harness],
        {
          detached: process.platform !== 'win32',
          stdio: 'ignore',
          shell: false,
        },
      )
      const kill = (signal) => {
        if (child.pid === undefined) return
        try {
          if (process.platform === 'win32') child.kill(signal)
          else process.kill(-child.pid, signal)
        } catch {
          // A concurrent child exit already releases this process group.
        }
      }
      const stop = () => {
        if (stopped) return
        stopped = true
        kill('SIGTERM')
        escalation = setTimeout(() => kill('SIGKILL'), this.options.killGraceMs ?? 1_000)
        escalation.unref()
      }
      this.terminate = stop
      const deadline = setTimeout(stop, this.options.deadlineMs ?? syncDeadlineMs)
      deadline.unref()
      const finish = (success) => {
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
