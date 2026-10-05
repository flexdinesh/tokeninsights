// Completion routing only. The native package entry point will supply the
// bounded CLI runner once collector sync and package tooling are integrated.
export type PiCompletionHost = {
  on(event: 'agent_settled', handler: (event: unknown) => Promise<void>): unknown
}

export function registerCompletion(pi: PiCompletionHost, sync: () => Promise<void>): void {
  pi.on('agent_settled', async () => {
    await sync()
  })
}
