import type { CollectorProgress as Progress } from '../contracts'

export const collectorStages = {
  waiting: 'Waiting to collect',
  capturing: 'Collecting usage',
  submitting: 'Submitting usage',
  accepted: 'Usage accepted',
  failed: 'Collection failed',
  interrupted: 'Collection interrupted',
}

// The dashboard controller owns polling for both progress and loading state.
export function CollectorProgress({ progress }: { progress: Progress | undefined }) {
  const attempts = progress?.attempts ?? []
  const active = attempts.filter((attempt) =>
    ['waiting', 'capturing', 'submitting'].includes(attempt.stage),
  )
  const terminal = attempts.reduce<(typeof attempts)[number] | undefined>((latest, attempt) => {
    if (['waiting', 'capturing', 'submitting'].includes(attempt.stage)) return latest
    return !latest || attempt.updatedAtMs >= latest.updatedAtMs ? attempt : latest
  }, undefined)
  const visible = terminal ? [...active, terminal] : active
  if (visible.length === 0) return null
  return (
    <section className="collector-progress" aria-label="Collector progress" aria-live="polite">
      {visible.map((attempt) => (
        <div key={attempt.attemptId}>
          <p role="status">
            {attempt.errorCode === 'submission_failed'
              ? 'Submission failed'
              : collectorStages[attempt.stage]}{' '}
            · {attempt.acknowledgedEntries.toLocaleString()} observations accepted
            {attempt.pendingKnown && attempt.pending > 0
              ? ` · ${attempt.pending.toLocaleString()} observations remaining`
              : ''}
          </p>
          {['waiting', 'capturing', 'submitting'].includes(attempt.stage) ? (
            <HarnessStates harnesses={attempt.harnesses} />
          ) : (
            <details>
              <summary>Harness collection</summary>
              <HarnessStates harnesses={attempt.harnesses} />
            </details>
          )}
          {(attempt.stage === 'failed' || attempt.stage === 'interrupted') && (
            <p role="alert">
              Saved usage remains available. Run tokeninsights sync to retry collection.
            </p>
          )}
        </div>
      ))}
    </section>
  )
}

function HarnessStates({ harnesses }: { harnesses: Progress['attempts'][number]['harnesses'] }) {
  return (
    <ul aria-label="Harness collection">
      {Object.entries(harnesses).map(([harness, state]) => (
        <li key={harness}>
          {harness}: {state}
        </li>
      ))}
    </ul>
  )
}
