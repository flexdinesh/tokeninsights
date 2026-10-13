// Notification routing: the caller owns background work and failure reporting.

export function registerCompletion(pi, sync) {
  pi.on('agent_settled', async () => {
    sync()
  })
}
