import { CollectorRunner, syncFailure } from '../../../tools/build/src/plugin-runner.ts'

export type PiCompletionContext = {
  hasUI: boolean
  ui: { notify(message: string, type: 'error'): void }
}
type PiHandler = (event: unknown, context: PiCompletionContext) => Promise<void>
export type PiLifecycleHost = {
  on(event: 'agent_settled', handler: PiHandler): unknown
  on(event: 'session_shutdown', handler: PiHandler): unknown
}

export function registerNativeCompletion(
  pi: PiLifecycleHost,
  runner = new CollectorRunner(),
): void {
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
