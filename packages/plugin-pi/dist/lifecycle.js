import { CollectorRunner, syncFailure } from './runner.js'

export function registerNativeCompletion(pi, runner = new CollectorRunner()) {
  pi.on('agent_settled', async (_event, context) => {
    try {
      await runner.run()
    } catch {
      if (context.hasUI) context.ui.notify(syncFailure, 'error')
    }
  })
  pi.on('session_shutdown', async () => {
    await runner.close()
  })
}
