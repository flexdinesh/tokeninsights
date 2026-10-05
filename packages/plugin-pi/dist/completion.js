// Completion routing only. The native package entry point will supply the
// bounded CLI runner once collector sync and package tooling are integrated.

export function registerCompletion(pi, sync) {
  pi.on('agent_settled', async () => {
    await sync()
  })
}
