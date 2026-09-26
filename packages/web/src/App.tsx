import { lazy, Suspense, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowDownToLine,
  Box,
  ChartNoAxesCombined,
  Check,
  CircleAlert,
  Command,
  Layers3,
  LoaderCircle,
  Monitor,
  Moon,
  GitBranch,
  RefreshCw,
  Sun,
} from 'lucide-react'
import { useAnalytics, useBootstrap, useFacets, useSyncStatus, syncNow } from './api'
import type { SyncStatus, Tab } from './contracts'
import { locationGroupSchema } from './contracts'
import { DashboardProvider, reduceQuery, searchFromQuery, useDashboardState } from './state'
import { labels } from './format'
import { FilterToolbar, QuickPeriods } from './components/Filters'
import { SummaryCards } from './components/SummaryCards'
import { ResultsTable } from './components/ResultsTable'
import { SyncCoverage } from './components/SyncCoverage'
import { SourceSelector } from './components/SourceSelector'
import { useSources } from './source-context'
import { createSource } from './sources'
import { Button } from './components/ui/button'
import { Skeleton } from './components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from './components/ui/alert'
import { Card } from './components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './components/ui/select'

const UsageChart = lazy(() =>
  import('./components/UsageChart').then((module) => ({ default: module.UsageChart })),
)

const tabs: { id: Tab; icon: typeof Activity }[] = [
  { id: 'tokens', icon: Activity },
  { id: 'models', icon: Box },
  { id: 'providers', icon: Layers3 },
  { id: 'harnesses', icon: Command },
  { id: 'sessions', icon: ChartNoAxesCombined },
  { id: 'context', icon: Layers3 },
  { id: 'repo', icon: GitBranch },
]

export function App() {
  const { active } = useSources()
  const bootstrap = useBootstrap(active.baseUrl)
  const defaults = active.defaults ?? bootstrap.data?.defaults
  if (!defaults) return <ConnectionScreen error={bootstrap.error} retry={bootstrap.refetch} />
  return (
    <DashboardProvider defaults={defaults}>
      <DashboardShell key={active.baseUrl} />
    </DashboardProvider>
  )
}

function ConnectionScreen({
  error,
  retry,
}: {
  error: Error | null
  retry: () => Promise<unknown>
}) {
  const { active } = useSources()
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <img src="/tokeninsights-logo.png" alt="" className="brand-mark" />
          <span>
            Token<span className="brand-light">Insights</span>
          </span>
        </div>
        <SourceSelector unavailable={Boolean(error)} />
      </header>
      <main className="startup-state">
        {error ? (
          <>
            <CircleAlert />
            <h1>Couldn’t connect to {active.hostname}</h1>
            <p>{error.message}</p>
            <Button onClick={() => void retry()}>Retry Connection</Button>
          </>
        ) : (
          <>
            <LoaderCircle className="spin" />
            <p>Connecting to {active.hostname}…</p>
          </>
        )}
      </main>
    </div>
  )
}

function DashboardShell() {
  const { active, add } = useSources()
  const {
    state: { query, theme },
    dispatch,
  } = useDashboardState()
  const client = useQueryClient()
  const bootstrapQuery = useBootstrap(active.baseUrl)
  const bootstrap = bootstrapQuery.data
  useEffect(() => {
    if (bootstrap) add(createSource(active.baseUrl, bootstrap))
  }, [active.baseUrl, add, bootstrap])
  const statusQuery = useSyncStatus(active.baseUrl, Boolean(bootstrap))
  const status = statusQuery.data
  const recoveryFailed = status?.phase === 'rebuild_failed'
  const enabled = Boolean(
    status && status.phase !== 'resetting' && status.phase !== 'rebuilding' && !recoveryFailed,
  )
  const revision = status?.revision ?? 0
  const analytics = useAnalytics(active.baseUrl, query, revision, enabled, Boolean(status?.running))
  const facets = useFacets(active.baseUrl, query, revision, enabled)
  const sync = useMutation({
    mutationFn: () => syncNow(active.baseUrl),
    onSuccess: (value) => {
      client.setQueryData(['sync', active.baseUrl], value)
    },
  })
  const reload = () => {
    void client.invalidateQueries({ queryKey: ['usage', active.baseUrl] })
    void client.invalidateQueries({ queryKey: ['facets', active.baseUrl] })
    void client.invalidateQueries({ queryKey: ['sync', active.baseUrl] })
    void client.invalidateQueries({ queryKey: ['instance', active.baseUrl] })
  }
  const running = status?.running || sync.isPending
  const data = analytics.data?.dashboard
  const hasData = Boolean(data && (!running || data.summary.syncedSessions > 0))
  const resultsMatchView =
    analytics.data?.tab === query.tab &&
    (query.tab !== 'repo' || analytics.data?.locationGroup === query.locationGroup)
  const sourceUnavailable = Boolean(
    bootstrapQuery.error || statusQuery.error || analytics.error || facets.error,
  )
  return (
    <div className="app-shell">
      <a href="#dashboard" className="skip-link">
        Skip to dashboard
      </a>
      <header className="app-header">
        <div className="brand">
          <img src="/tokeninsights-logo.png" alt="" className="brand-mark" />
          <span>
            Token<span className="brand-light">Insights</span>
          </span>
        </div>
        <div className="header-actions">
          <SourceSelector unavailable={sourceUnavailable} />
          <span className="header-status" role="status">
            {sourceUnavailable
              ? 'Unavailable'
              : running
                ? 'Syncing…'
                : data?.lastSynced
                  ? 'Synced'
                  : 'Ready'}
          </span>
          <span className="header-divider" />
          <Button
            variant="ghost"
            size="icon"
            className="theme-button"
            title={`Theme: ${theme}`}
            aria-label={`Theme: ${theme}. Change theme`}
            onClick={() =>
              dispatch({
                type: 'theme',
                value: theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system',
              })
            }
          >
            {theme === 'dark' ? (
              <Moon size="1.1em" />
            ) : theme === 'light' ? (
              <Sun size="1.1em" />
            ) : (
              <Monitor size="1.1em" />
            )}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="reload-button"
            aria-label="Reload Data"
            title="Reload Data"
            disabled={running || !bootstrap}
            onClick={reload}
          >
            <RefreshCw size="1em" className={analytics.isFetching && enabled ? 'spin' : ''} />
            <span>Reload Data</span>
          </Button>
          <Button
            className="sync-button"
            size="sm"
            disabled={running || !bootstrap}
            onClick={() => sync.mutate()}
          >
            {running ? (
              <LoaderCircle size="1em" className="spin" />
            ) : (
              <ArrowDownToLine size="1em" />
            )}
            <span>{running ? 'Syncing…' : 'Sync Usage'}</span>
          </Button>
        </div>
      </header>
      {!bootstrap && (
        <main id="dashboard" className="startup-state">
          {bootstrapQuery.error ? (
            <>
              <CircleAlert />
              <h1>Couldn’t connect to {active.hostname}</h1>
              <p>{bootstrapQuery.error.message}</p>
              <Button onClick={() => void bootstrapQuery.refetch()}>Retry Connection</Button>
            </>
          ) : (
            <>
              <LoaderCircle className="spin" />
              <p>Connecting to {active.hostname}…</p>
            </>
          )}
        </main>
      )}
      {bootstrap && (
        <main id="dashboard" className="dashboard">
          <h1 className="sr-only">Token usage</h1>
          <div className="view-controls">
            <nav className="view-tabs" aria-label="Analytics views">
              {tabs.map(({ id, icon: Icon }) => (
                <Button key={id} variant="ghost" asChild>
                  <Link
                    to="/$tab"
                    params={{ tab: id }}
                    search={searchFromQuery(reduceQuery(query, { type: 'tab', value: id }))}
                    activeOptions={{ exact: true, includeSearch: false }}
                  >
                    <Icon size="1em" />
                    {labels[id]}
                  </Link>
                </Button>
              ))}
            </nav>
            <QuickPeriods />
          </div>
          {query.tab === 'repo' && (
            <div className="repo-controls" aria-label="Repo view options">
              <div className="bucket-control">
                <span>Group by</span>
                <Select
                  value={query.locationGroup}
                  onValueChange={(value) => {
                    const parsed = locationGroupSchema.safeParse(value)
                    if (parsed.success) dispatch({ type: 'locationGroup', value: parsed.data })
                  }}
                >
                  <SelectTrigger aria-label="Group by location">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {locationGroupSchema.options.map((value) => (
                      <SelectItem key={value} value={value}>
                        {value[0]?.toUpperCase() + value.slice(1)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
          <div className="studio-layout">
            <FilterToolbar
              baseUrl={active.baseUrl}
              facets={facets.data}
              revision={revision}
              enabled={enabled}
            />
            <div className="studio-results">
              {(bootstrapQuery.error || statusQuery.error || sync.error) && (
                <ErrorBanner
                  message={
                    bootstrapQuery.error?.message ??
                    statusQuery.error?.message ??
                    sync.error?.message ??
                    'Request failed'
                  }
                  onRetry={() => {
                    if (bootstrapQuery.error) void bootstrapQuery.refetch()
                    else if (sync.error) sync.mutate()
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
                  <Button variant="outline" onClick={() => sync.mutate()} disabled={running}>
                    Retry Sync
                  </Button>
                </Alert>
              )}
              {enabled && analytics.error && !running && (
                <ErrorBanner message={analytics.error.message} onRetry={reload} />
              )}
              {enabled && facets.error && !running && (
                <ErrorBanner
                  message="Filter values couldn’t load."
                  onRetry={() => void facets.refetch()}
                />
              )}
              {enabled && data?.coverage && (
                <SyncCoverage days={data.coverage} timezone={bootstrap.timezone} />
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
                          <span
                            className="sr-only"
                            role="status"
                            aria-label="Updating view results"
                          >
                            Updating view results
                          </span>
                        )}
                        <Suspense
                          fallback={
                            <Skeleton
                              className="skeleton-chart"
                              role="status"
                              aria-label="Loading chart"
                            />
                          }
                        >
                          <UsageChart rows={data.chart} totalTokens={data.summary.total} />
                        </Suspense>
                      </>
                    )}
                  </div>
                  {(!analytics.isPlaceholderData || resultsMatchView) && (
                    <ResultsTable data={data} timezone={bootstrap.timezone} />
                  )}
                </div>
              )}
            </div>
          </div>
          <footer className="app-footer">
            <span>
              <span className="status-dot" />
              {active.hostname} · {active.baseUrl}
            </span>
            <span>OpenCode · Pi · Codex · Claude Code</span>
          </footer>
        </main>
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
