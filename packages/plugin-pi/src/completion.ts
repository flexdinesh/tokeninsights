// Notification routing: the caller owns background work and failure reporting.
export type PiCompletionHost = {
  on(event: 'agent_settled', handler: (event: unknown) => Promise<void>): unknown
}

export function registerCompletion(pi: PiCompletionHost, sync: () => void): void {
  pi.on('agent_settled', async () => {
    sync()
  })
}
