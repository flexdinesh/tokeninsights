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
  const { statusQuery, reload, analyticsEnabled: enabled } = controller
  const data = analytics.data?.dashboard
  const hasData = Boolean(data)
  const resultsMatchView =
    analytics.data?.tab === query.tab &&
    (query.tab !== 'repo' || analytics.data?.locationGroup === query.locationGroup)
  return (
    <div className="studio-results">
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
      {enabled && !hasData && !analytics.error && <DashboardSkeleton />}
      {enabled && data && data.summary.syncedSessions === 0 && !analytics.error && (
        <p role="status">No usage saved yet. Run tokeninsights sync.</p>
      )}
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
