import { CollectorRunner, syncFailure } from './runner.js'
import { isCompletion } from './completion.js'

export function setupCompletion(context) {
  const controller = new AbortController()
  const runner = new CollectorRunner()
  let reported
  const report = () => {
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
