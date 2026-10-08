import { CollectorRunner, syncFailure } from '../../../tools/build/src/plugin-runner.ts'
import { isCompletion } from './completion.ts'

type CompletionContext = {
  event: { subscribe(options: { signal: AbortSignal }): AsyncIterable<unknown> }
}

export function setupCompletion(context: CompletionContext): () => Promise<void> {
  const controller = new AbortController()
  const runner = new CollectorRunner({ harness: 'opencode' })
  let reported: Promise<void> | undefined
  const report = (): void => {
    // OpenCode owns plugin console logs. Do not log events or subprocess output.
    if (!controller.signal.aborted) console.error(syncFailure)
  }
  const subscription = (async () => {
    try {
      for await (const event of context.event.subscribe({ signal: controller.signal })) {
        if (controller.signal.aborted) break
        if (isCompletion(event)) {
          const pending = runner.run()
          if (reported !== pending) {
            reported = pending
            void pending.catch(report).finally(() => {
              if (reported === pending) reported = undefined
            })
          }
        }
      }
    } catch {
      report()
    }
  })()
  return async () => {
    controller.abort()
    await runner.close()
    await subscription
  }
}
