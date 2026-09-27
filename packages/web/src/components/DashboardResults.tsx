import { Check, CircleAlert, LoaderCircle } from 'lucide-react'
import type { useAnalytics, useFacets } from '../api'
import type { SyncStatus } from '../contracts'
import { useDashboardQuery } from '../state'
import type { useDashboardSync } from '../useDashboardSync'
import { ChartPanel } from './ChartPanel'
import { SummaryCards } from './SummaryCards'
import { ResultsTable } from './ResultsTable'
import { SyncCoverage } from './SyncCoverage'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { Card } from './ui/card'

export function DashboardResults({
  analytics,
  facets,
  controller,
  timezone,
  connectionError,
  retryConnection,
}: {
  analytics: ReturnType<typeof useAnalytics>
  facets: ReturnType<typeof useFacets>
  controller: ReturnType<typeof useDashboardSync>
  timezone: string
  connectionError: Error | null
  retryConnection: () => Promise<unknown>
}) {
  const { query } = useDashboardQuery()
  const {
    statusQuery,
    sync,
    startSync,
    reload,
    running,
    checkedSince,
    analyticsEnabled: enabled,
  } = controller
  const status = statusQuery.data
  const data = analytics.data?.dashboard
  const hasData = Boolean(data && (!running || data.summary.syncedSessions > 0))
  const resultsMatchView =
    analytics.data?.tab === query.tab &&
    (query.tab !== 'repo' || analytics.data?.locationGroup === query.locationGroup)
  return (
    <div className="studio-results">
      {(connectionError || statusQuery.error || sync.error) && (
        <ErrorBanner
          message={
            connectionError?.message ??
            statusQuery.error?.message ??
            sync.error?.message ??
            'Request failed'
          }
          onRetry={() => {
            if (connectionError) void retryConnection()
            else if (sync.error) startSync()
            else void statusQuery.refetch()
          }}
        />
      )}
      {status?.running && <SyncProgress status={status} hasData={hasData} />}
      {status?.error && (
        <Alert className="sync-error" variant="destructive" role="alert">
          <CircleAlert size="1.3em" />
          <AlertDescription>
            <AlertTitle>Sync needs attention</AlertTitle>
            <p>{status.error}</p>
          </AlertDescription>
          <Button variant="outline" onClick={() => startSync()} disabled={running}>
            Retry Sync
          </Button>
        </Alert>
      )}
      {enabled && analytics.error && !running && (
        <ErrorBanner message={analytics.error.message} onRetry={reload} />
      )}
      {enabled && facets.error && !running && (
        <ErrorBanner message="Filter values couldn’t load." onRetry={() => void facets.refetch()} />
      )}
      {enabled && !hasData && (!analytics.error || running) && <DashboardSkeleton />}
      {enabled && data && hasData && (
        <div className="analytics" aria-busy={analytics.isFetching}>
          <div className="usage-workbench">
            <SummaryCards summary={data.summary} />
            {analytics.isPlaceholderData && !resultsMatchView ? (
              <RouteResultsSkeleton />
            ) : (
              <>
                {analytics.isPlaceholderData && (
                  <span className="sr-only" role="status" aria-label="Updating view results">
                    Updating view results
                  </span>
                )}
                <ChartPanel rows={data.chart} totalTokens={data.summary.total} />
              </>
            )}
          </div>
          {(!analytics.isPlaceholderData || resultsMatchView) && (
            <ResultsTable data={data} timezone={timezone} checkedSince={checkedSince} />
          )}
        </div>
      )}
      {enabled && data?.coverage && (
        <SyncCoverage days={data.coverage} timezone={timezone} checkedSince={checkedSince} />
      )}
    </div>
  )
}

function RouteResultsSkeleton() {
  return (
    <div role="status" aria-label="Updating view results">
      <Skeleton className="skeleton-chart" />
      <Skeleton className="skeleton-table" />
    </div>
  )
}

function SyncProgress({ status, hasData }: { status: SyncStatus; hasData: boolean }) {
  const message =
    status.phase === 'resetting'
      ? 'Resetting usage data for compatibility…'
      : status.phase === 'rebuilding'
        ? 'Rebuilding usage from all configured harnesses…'
        : status.phase === 'normalizing'
          ? 'Normalizing canonical data…'
          : hasData
            ? 'Syncing all supported harnesses. Saved usage remains available while this runs.'
            : 'Syncing all supported harnesses…'
  return (
    <Card className="sync-progress panel" role="status" aria-live="polite">
      <div className="panel-heading">
        <div>
          <h2>
            <LoaderCircle className="spin" size="1em" />
            Refreshing usage
          </h2>
          <p>{message}</p>
        </div>
      </div>
      {status.progress && (
        <div className="sync-work-progress">
          <progress
            aria-label="Sources checked"
            max={
              status.progress.discoveryComplete
                ? Math.max(1, status.progress.totalSources)
                : undefined
            }
            value={status.progress.discoveryComplete ? status.progress.checkedSources : undefined}
          />
          <span>
            {status.progress.discoveryComplete
              ? `${status.progress.checkedSources} / ${status.progress.totalSources} sources checked`
              : `${status.progress.totalSources} sources found · discovering`}
          </span>
          {status.progress.discoveryComplete && status.progress.totalSources > 0 && (
            <span>
              {Math.floor((status.progress.checkedSources / status.progress.totalSources) * 100)}%
              checked
            </span>
          )}
          <span>
            {status.progress.readySources} ready
            {status.progress.failedSources > 0 ? ` · ${status.progress.failedSources} failed` : ''}
          </span>
          <span>
            {Math.max(
              0,
              Math.floor((status.progress.updatedAt - status.progress.startedAt) / 1000),
            )}
            s elapsed
          </span>
        </div>
      )}
      <div className="sync-harnesses">
        {Object.entries(status.harnesses).map(([harness, phase]) => (
          <div key={harness} data-phase={phase}>
            <span>
              {phase === 'synced' || phase === 'skipped' ? (
                <Check size="1em" />
              ) : phase === 'failed' ? (
                <CircleAlert size="1em" />
              ) : (
                <span className={`status-dot ${phase === 'pending' ? 'pending' : ''}`} />
              )}
              {harness}
            </span>
            <small>{phase}</small>
          </div>
        ))}
      </div>
    </Card>
  )
}

function ErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <Alert className="error-banner" variant="destructive" role="alert">
      <CircleAlert size="1.1em" />
      <AlertDescription>{message}</AlertDescription>
      <Button variant="outline" onClick={onRetry}>
        Retry Request
      </Button>
    </Alert>
  )
}

function DashboardSkeleton() {
  return (
    <div className="dashboard-skeleton" role="status" aria-label="Loading dashboard">
      <div className="summary-grid">
        {[1, 2, 3, 4, 5].map((n) => (
          <Skeleton key={n} className="skeleton-card" />
        ))}
      </div>
      <Skeleton className="skeleton-chart" />
      <span className="sr-only">Loading dashboard…</span>
    </div>
  )
}
