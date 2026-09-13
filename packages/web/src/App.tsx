import { lazy, Suspense, useEffect, useState } from 'react'
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
  RefreshCw,
  Sun,
} from 'lucide-react'
import { useAnalytics, useBootstrap, useFacets, useSyncStatus, syncNow } from './api'
import type { SyncStatus, Tab } from './contracts'
import { DashboardProvider, reduceQuery, searchFromQuery, useDashboardState } from './state'
import { labels } from './format'
import { FilterToolbar, QuickPeriods } from './components/Filters'
import { SummaryCards } from './components/SummaryCards'
import { ResultsTable } from './components/ResultsTable'
import { SourceSelector } from './components/SourceSelector'
import { useSources } from './source-context'
import { createSource } from './sources'
import { Button } from './components/ui/button'
import { Skeleton } from './components/ui/skeleton'
import { Alert, AlertDescription, AlertTitle } from './components/ui/alert'
import { Card } from './components/ui/card'

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
  const [inspectedRevision, setInspectedRevision] = useState<number | null>(null)
  const recoveryFailed = status?.phase === 'rebuild_failed'
  const enabled = Boolean(
    status &&
    !status.running &&
    !recoveryFailed &&
    (!status.error || inspectedRevision === status.revision),
  )
  const revision = status?.revision ?? 0
  const analytics = useAnalytics(active.baseUrl, query, revision, enabled)
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
  const data = analytics.data
  return (
    <div className="app-shell">
      <a href="#dashboard" className="skip-link">
        Skip to dashboard
      </a>
      <header className="app-header">
        <div className="brand">
          <span>
            Token<span className="brand-light">Insights</span>
          </span>
        </div>
        <div className="header-actions">
          <SourceSelector
            unavailable={Boolean(
              bootstrapQuery.error || statusQuery.error || analytics.error || facets.error,
            )}
          />
          <span className="header-divider" />
          <Button
            variant="ghost"
            size="icon"
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
            className="reload-button"
            aria-label="Reload Data"
            title="Reload Data"
            disabled={running || !bootstrap}
            onClick={reload}
          >
            <RefreshCw size="1em" className={analytics.isFetching && enabled ? 'spin' : ''} />
            <span>Reload Data</span>
          </Button>
          <Button disabled={running || !bootstrap} onClick={() => sync.mutate()}>
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
          <FilterToolbar
            baseUrl={active.baseUrl}
            facets={facets.data}
            revision={revision}
            enabled={enabled}
          />
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
          {status?.running && <SyncProgress status={status} />}
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
              {!enabled && !recoveryFailed && (
                <Button variant="outline" onClick={() => setInspectedRevision(status.revision)}>
                  Inspect Existing Data
                </Button>
              )}
            </Alert>
          )}
          {enabled && analytics.error && (
            <ErrorBanner message={analytics.error.message} onRetry={reload} />
          )}
          {enabled && facets.error && (
            <ErrorBanner
              message="Filter values couldn’t load."
              onRetry={() => void facets.refetch()}
            />
          )}
          {enabled && !data && !analytics.error && <DashboardSkeleton />}
          {enabled && data && (
            <div className="analytics" aria-busy={analytics.isFetching}>
              <SummaryCards summary={data.summary} />
              {analytics.isPlaceholderData ? (
                <RouteResultsSkeleton />
              ) : (
                <>
                  <Suspense
                    fallback={
                      <Skeleton
                        className="skeleton-chart"
                        role="status"
                        aria-label="Loading chart"
                      />
                    }
                  >
                    <UsageChart rows={data.chart} />
                  </Suspense>
                  <ResultsTable data={data} />
                </>
              )}
            </div>
          )}
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

function SyncProgress({ status }: { status: SyncStatus }) {
  const message =
    status.phase === 'resetting'
      ? 'Resetting usage data for compatibility…'
      : status.phase === 'rebuilding'
        ? 'Rebuilding usage from all configured harnesses…'
        : status.phase === 'normalizing'
          ? 'Normalizing canonical data…'
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
