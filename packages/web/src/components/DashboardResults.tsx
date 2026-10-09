import { CircleAlert } from 'lucide-react'
import type { useAnalytics, useFacets } from '../api'
import { useDashboardQuery } from '../state'
import type { useDashboardSync } from '../useDashboardSync'
import { ChartPanel } from './ChartPanel'
import { SummaryCards } from './SummaryCards'
import { ResultsTable } from './ResultsTable'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'
import { Alert, AlertDescription } from './ui/alert'
import { formatServerDateTime } from '../format'

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
  const { query, setQuality } = useDashboardQuery()
  const { statusQuery, reload, analyticsEnabled: enabled } = controller
  const received = analytics.data?.dashboard
  const data = (received?.quality ?? 'confirmed') === query.quality ? received : undefined
  const hasData = Boolean(data)
  const waitingForFirstData = controller.loading && data?.summary.syncedSessions === 0
  const failed = statusQuery.data?.failed ?? 0
  const retryAt = statusQuery.data?.failedRetryAtMs ?? 0
  const resultsMatchView =
    analytics.data?.tab === query.tab &&
    (query.tab !== 'repo' || analytics.data?.locationGroup === query.locationGroup)
  return (
    <div className="studio-results">
      <div role="group" aria-label="Usage evidence">
        <Button
          variant={query.quality === 'confirmed' ? 'default' : 'outline'}
          aria-pressed={query.quality === 'confirmed'}
          onClick={() => setQuality('confirmed')}
        >
          Confirmed
        </Button>
        <Button
          variant={query.quality === 'estimated' ? 'default' : 'outline'}
          aria-pressed={query.quality === 'estimated'}
          onClick={() => setQuality('estimated')}
        >
          Estimated
        </Button>
      </div>
      {query.quality === 'estimated' && (
        <p>Ambiguous evidence with usable counters. Excluded from confirmed totals.</p>
      )}
      {controller.processingPending && (
        <p role="status">
          {statusQuery.data?.generation !== statusQuery.data?.targetGeneration
            ? 'Rebuilding saved usage. Existing totals remain available.'
            : `Processing usage · ${statusQuery.data?.pending.toLocaleString()} pending.`}
        </p>
      )}
      {failed > 0 && (
        <p role="status">
          {controller.processingBackoff
            ? `Some usage needs a processing retry. Retry scheduled at ${formatServerDateTime(retryAt, timezone)}.`
            : 'Retrying usage processing.'}{' '}
          Saved totals remain available.
        </p>
      )}
      {(data?.unresolved ?? 0) > 0 && <p>{data?.unresolved} observations need more evidence.</p>}
      {(connectionError || statusQuery.error) && (
        <ErrorBanner
          message={connectionError?.message ?? statusQuery.error?.message ?? 'Request failed'}
          onRetry={() => {
            if (connectionError) void retryConnection()
            else void statusQuery.refetch()
          }}
        />
      )}
      {enabled && analytics.error && (
        <ErrorBanner message={analytics.error.message} onRetry={reload} />
      )}
      {enabled && facets.error && (
        <ErrorBanner message="Filter values couldn’t load." onRetry={() => void facets.refetch()} />
      )}
      {controller.collectionUnknown && (
        <ErrorBanner
          message="Collection status couldn’t load. Saved usage remains available."
          onRetry={() => void controller.progressQuery.refetch()}
        />
      )}
      {enabled && (!hasData || waitingForFirstData) && !analytics.error && <DashboardSkeleton />}
      {enabled &&
        data &&
        data.summary.syncedSessions === 0 &&
        !analytics.error &&
        !controller.loading &&
        !controller.collectionFailed &&
        !controller.collectionUnknown &&
        failed === 0 && (
          <p role="status">
            {query.quality === 'estimated'
              ? 'No estimated usage.'
              : 'No usage saved yet. Run tokeninsights sync.'}
          </p>
        )}
      {enabled && data && hasData && !waitingForFirstData && (
        <div className="analytics" aria-busy={analytics.isFetching}>
          <div className="usage-workbench">
            <SummaryCards summary={data.summary} estimated={query.quality === 'estimated'} />
            {analytics.isPlaceholderData && !resultsMatchView ? (
              <RouteResultsSkeleton />
            ) : (
              <>
                {analytics.isPlaceholderData && (
                  <span className="sr-only" role="status" aria-label="Updating view results">
                    Updating view results
                  </span>
                )}
                <ChartPanel
                  rows={data.chart}
                  totalTokens={data.summary.total}
                  timezone={timezone}
                />
              </>
            )}
          </div>
          {(!analytics.isPlaceholderData || resultsMatchView) && (
            <ResultsTable data={data} timezone={timezone} />
          )}
        </div>
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
