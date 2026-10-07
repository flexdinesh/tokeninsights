import { useCollectorProgress } from '../api'

const stages = {
  waiting: 'Waiting to collect',
  capturing: 'Collecting usage',
  submitting: 'Submitting usage',
  accepted: 'Usage accepted',
  failed: 'Collection failed',
  interrupted: 'Collection interrupted',
}

// Mounted only for personal servers advertising collector-progress.
export function CollectorProgress({ instanceId }: { instanceId: string }) {
  const query = useCollectorProgress(true, instanceId)
  const attempts = query.data?.attempts ?? []
  if (attempts.length === 0) return null
  return (
    <section className="collector-progress" aria-label="Collector progress" aria-live="polite">
      {attempts.map((attempt) => (
        <p key={attempt.attemptId} role="status">
          {stages[attempt.stage]} · {attempt.acknowledgedEntries.toLocaleString()} observations
          accepted
          {attempt.pendingKnown && attempt.pending > 0
            ? ` · ${attempt.pending.toLocaleString()} observations remaining`
            : ''}
        </p>
      ))}
    </section>
  )
}
