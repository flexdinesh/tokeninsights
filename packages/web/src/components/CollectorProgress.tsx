import type { CollectorProgress as Progress } from '../contracts'

export const collectorStages = {
  waiting: 'Waiting to collect',
  capturing: 'Collecting usage',
  submitting: 'Submitting usage',
  accepted: 'Usage accepted',
  failed: 'Collection failed',
  interrupted: 'Collection interrupted',
}

const harnessStates = {
  waiting: 'Waiting',
  running: 'Collecting',
  complete: 'Collected',
  failed: 'Collection failed',
  skipped: 'No new usage',
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
          {attempt.stage !== 'accepted' && <AttemptStatus attempt={attempt} />}
          {['waiting', 'capturing', 'submitting'].includes(attempt.stage) ? (
            <HarnessStates harnesses={attempt.harnesses} />
          ) : (
            <details>
              <summary>Latest collection</summary>
              {attempt.stage === 'accepted' && <AttemptStatus attempt={attempt} />}
              <HarnessStates harnesses={attempt.harnesses} />
            </details>
          )}
          {(attempt.stage === 'failed' || attempt.stage === 'interrupted') && (
            <p role="alert">
              Saved usage remains available. Restart tokeninsights web to retry collection.
            </p>
          )}
        </div>
      ))}
    </section>
  )
}

function AttemptStatus({ attempt }: { attempt: Progress['attempts'][number] }) {
  return (
    <p role="status">
      {attempt.errorCode === 'submission_failed'
        ? 'Submission failed'
        : collectorStages[attempt.stage]}{' '}
      · {attempt.acknowledgedEntries.toLocaleString()} observations accepted
      {attempt.pendingKnown && attempt.pending > 0
        ? ` · ${attempt.pending.toLocaleString()} observations remaining`
        : ''}
    </p>
  )
}

function HarnessStates({ harnesses }: { harnesses: Progress['attempts'][number]['harnesses'] }) {
  return (
    <>
      <p>Status for this run only; saved usage remains included.</p>
      <ul aria-label="Collection by harness">
        {Object.entries(harnesses).map(([harness, state]) => (
          <li key={harness}>
            {harness}: {harnessStates[state]}
          </li>
        ))}
      </ul>
      {Object.values(harnesses).includes('skipped') && (
        <p>No new usage means sources were unchanged or none were found.</p>
      )}
    </>
  )
}
