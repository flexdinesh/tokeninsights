import { Heading } from '@/ui'
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
    <section
      className="mt-3 grid gap-3 text-label text-ink-subtle [&_p]:my-1"
      aria-label="Collector progress"
    >
      {visible.map((attempt) => (
        <div key={attempt.attemptId}>
          {!['waiting', 'capturing', 'submitting'].includes(attempt.stage) && (
            <Heading as="h3" variant="sub" className="text-ink">
              Latest collection
            </Heading>
          )}
          <AttemptStatus attempt={attempt} />
          <HarnessStates harnesses={attempt.harnesses} />
        </div>
      ))}
    </section>
  )
}

function AttemptStatus({ attempt }: { attempt: Progress['attempts'][number] }) {
  return (
    <p>
      {attempt.errorCode === 'submission_failed'
        ? 'Submission failed'
        : collectorStages[attempt.stage]}{' '}
      · {attempt.acknowledgedEntries.toLocaleString()} accepted
      {attempt.pendingKnown && attempt.pending > 0
        ? ` · ${attempt.pending.toLocaleString()} remaining`
        : ''}
    </p>
  )
}

function HarnessStates({ harnesses }: { harnesses: Progress['attempts'][number]['harnesses'] }) {
  return (
    <>
      <ul
        className="my-1 flex flex-wrap gap-x-4 gap-y-2"
        aria-label="Collection by harness"
        title="This run only; saved usage remains included."
      >
        {Object.entries(harnesses).map(([harness, state]) => (
          <li
            key={harness}
            title={
              state === 'skipped'
                ? 'Sources unchanged or none found; saved usage included.'
                : undefined
            }
          >
            {harness}: {harnessStates[state]}
          </li>
        ))}
      </ul>
    </>
  )
}
