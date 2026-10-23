import { useEffect, useRef } from 'react'
import { ArrowLeft, ArrowRight, CircleAlert } from 'lucide-react'
import type { useAnalytics, useFacets } from '../api'
import { useDashboardQuery } from '../state'
import type { useDashboardSync } from '../useDashboardSync'
import { ChartPanel } from './ChartPanel'
import { SummaryCards } from './SummaryCards'
import { ResultsTable } from './ResultsTable'
import { exactCount, formatCount, formatServerDateTime } from '../format'
import { Alert, AlertDescription, Button, Heading, Skeleton, Text } from '@/ui'

export function DashboardResults({
  analytics,
  excludedUsage,
  facets,
  controller,
  timezone,
  localMachine,
  connectionError,
  retryConnection,
}: {
  analytics: ReturnType<typeof useAnalytics>
  excludedUsage: ReturnType<typeof useAnalytics>
  facets: ReturnType<typeof useFacets>
  controller: ReturnType<typeof useDashboardSync>
  timezone: string
  localMachine: boolean
  connectionError: Error | null
  retryConnection: () => Promise<unknown>
}) {
  const { query, setQuality } = useDashboardQuery()
  const reviewingExcluded = query.quality === 'estimated'
  const resultsRef = useRef<HTMLDivElement>(null)
  const previousQuality = useRef(query.quality)
  useEffect(() => {
    if (previousQuality.current === query.quality) return
    previousQuality.current = query.quality
    resultsRef.current?.focus({ preventScroll: true })
  }, [query.quality])
  const { statusQuery, analyticsEnabled: enabled } = controller
  const received = analytics.data?.dashboard
  const data = (received?.quality ?? 'confirmed') === query.quality ? received : undefined
  const hasData = Boolean(data)
  const excluded = excludedUsage.data?.dashboard
  const hasExcludedUsage =
    excluded?.quality === 'estimated' &&
    excluded.revision === data?.revision &&
    excluded.summary.sessions > 0
  const waitingForFirstData = controller.loading && data?.summary.syncedSessions === 0
  const failed = statusQuery.data?.failed ?? 0
  const retryAt = statusQuery.data?.failedRetryAtMs ?? 0
  const resultsMatchView =
    analytics.data?.tab === query.tab &&
    (query.tab !== 'repo' || analytics.data?.locationGroup === query.locationGroup)
  return (
    <div
      className="min-w-0"
      ref={resultsRef}
      tabIndex={-1}
      role="region"
      aria-label={reviewingExcluded ? 'Excluded usage results' : 'Usage results'}
    >
      <span className="sr-only" role="status">
        {reviewingExcluded
          ? 'Showing excluded usage. These counts stay outside your usage total.'
          : 'Showing usage totals.'}
      </span>
      {reviewingExcluded && (
        <div className="border-b border-line py-3">
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            <Heading>Excluded usage</Heading>
            <Button variant="ghost" onClick={() => setQuality('confirmed')}>
              <ArrowLeft aria-hidden="true" />
              Back to usage
            </Button>
          </div>
          <Text variant="secondary" className="mt-1 max-w-prose">
            These saved token counts can’t be confidently included in your usage total. They stay
            separate. Your date range and filters apply to this review.
          </Text>
        </div>
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
        <ErrorBanner message={analytics.error.message} onRetry={() => void analytics.refetch()} />
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
      {controller.collectionFailed && (
        <p role="alert">
          Saved usage remains available. Restart tokeninsights web to retry collection.
        </p>
      )}
      {enabled && (!hasData || waitingForFirstData) && !analytics.error && <DashboardSkeleton />}
      {enabled &&
        data &&
        data.summary.syncedSessions === 0 &&
        !analytics.error &&
        !controller.loading &&
        !controller.collectionFailed &&
        !controller.collectionUnknown &&
        (reviewingExcluded || !excludedUsage.isPending) &&
        failed === 0 && (
          <p role="status">
            {query.quality === 'estimated'
              ? 'No excluded usage saved.'
              : hasExcludedUsage || excludedUsage.error
                ? 'No usage included in your total yet.'
                : localMachine
                  ? 'No usage saved yet. Restart tokeninsights web to collect usage.'
                  : 'No usage saved yet. Run tokeninsights sync.'}
          </p>
        )}
      {enabled && data && hasData && !waitingForFirstData && (
        <div aria-busy={analytics.isFetching}>
          <div className="mb-4">
            <SummaryCards summary={data.summary} estimated={query.quality === 'estimated'} />
            {!reviewingExcluded && hasExcludedUsage && excluded && (
              <div className={excludedNotice}>
                <p>
                  Usage excluded from totals
                  <span title={`${exactCount(excluded.summary.total)} tokens`}>
                    {' '}
                    · {formatCount(excluded.summary.total)} tokens
                  </span>
                </p>
                <Button
                  variant="ghost"
                  size="compact"
                  className="min-w-0 shrink whitespace-normal"
                  onClick={() => setQuality('estimated')}
                >
                  Review excluded usage
                  <ArrowRight aria-hidden="true" />
                </Button>
              </div>
            )}
            {!reviewingExcluded && excludedUsage.error && (
              <div className={excludedNotice} role="status">
                <p>Excluded usage couldn’t load.</p>
                <Button variant="ghost" size="compact" onClick={() => void excludedUsage.refetch()}>
                  Retry excluded usage
                </Button>
              </div>
            )}
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
            <ResultsTable data={data} timezone={timezone} hasExcludedUsage={hasExcludedUsage} />
          )}
        </div>
      )}
    </div>
  )
}

const excludedNotice =
  'flex flex-wrap items-center gap-x-4 gap-y-2 py-2 text-label [&>p]:text-ink-muted'

function RouteResultsSkeleton() {
  return (
    <div role="status" aria-label="Updating view results">
      <Skeleton className="h-(--skeleton-chart-height)" />
      <Skeleton className="mt-2 h-(--skeleton-table-height)" />
    </div>
  )
}

function ErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <Alert className="my-3" variant="destructive" role="alert">
      <CircleAlert aria-hidden="true" />
      <AlertDescription>{message}</AlertDescription>
      <Button variant="outline" onClick={onRetry}>
        Retry Request
      </Button>
    </Alert>
  )
}

function DashboardSkeleton() {
  return (
    <div role="status" aria-label="Loading dashboard">
      <div className="mb-2 grid grid-cols-(--readout-columns) gap-4 py-2 max-compact:grid-cols-(--readout-columns-narrow)">
        {[1, 2, 3, 4, 5].map((n) => (
          <Skeleton key={n} className="h-(--skeleton-readout-height)" />
        ))}
      </div>
      <Skeleton className="h-(--skeleton-chart-height)" />
      <span className="sr-only">Loading dashboard…</span>
    </div>
  )
}
