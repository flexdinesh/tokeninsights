import { lazy, Suspense, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Activity, ArrowDownToLine, Box, ChartNoAxesCombined, Check, CircleAlert, Command, Layers3, LoaderCircle, Monitor, Moon, RefreshCw, Sun } from 'lucide-react'
import { useAnalytics, useBootstrap, useFacets, useSyncStatus, syncNow } from './api'
import type { Bootstrap, SyncStatus, Tab } from './contracts'
import { DashboardProvider, useDashboardState } from './state'
import { labels, relativeTime } from './format'
import { FilterToolbar, QuickPeriods } from './components/Filters'
import { SummaryCards } from './components/SummaryCards'
import { ResultsTable } from './components/ResultsTable'

const UsageChart = lazy(() => import('./components/UsageChart').then(module => ({ default: module.UsageChart })))

const tabs: { id: Tab; icon: typeof Activity }[] = [
  { id: 'tokens', icon: Activity }, { id: 'models', icon: Box }, { id: 'providers', icon: Layers3 },
  { id: 'harnesses', icon: Command }, { id: 'sessions', icon: ChartNoAxesCombined }, { id: 'context', icon: Layers3 },
]

export function App() {
  const bootstrap = useBootstrap()
  if (bootstrap.error) return <main className="startup-state"><CircleAlert /><h1>Couldn’t connect</h1><p>{bootstrap.error.message}</p><button className="button primary" onClick={() => void bootstrap.refetch()}>Retry</button></main>
  if (!bootstrap.data) return <main className="startup-state"><LoaderCircle className="spin" /><p>Opening TokenInsights…</p></main>
  return <DashboardProvider defaults={bootstrap.data.defaults}><DashboardShell bootstrap={bootstrap.data} /></DashboardProvider>
}

function DashboardShell({ bootstrap }: { bootstrap: Bootstrap }) {
  const { state: { query, theme }, dispatch } = useDashboardState()
  const client = useQueryClient()
  const statusQuery = useSyncStatus()
  const status = statusQuery.data
  const [inspectedRevision, setInspectedRevision] = useState<number | null>(null)
  const recoveryFailed = status?.phase === 'rebuild_failed'
  const enabled = Boolean(status && !status.running && !recoveryFailed && (!status.error || inspectedRevision === status.revision))
  const revision = status?.revision ?? 0
  const analytics = useAnalytics(query, revision, enabled)
  const facets = useFacets(query, revision, enabled)
  const sync = useMutation({ mutationFn: syncNow, onSuccess: value => { client.setQueryData(['status'], value) } })
  const reload = () => { void client.invalidateQueries({ queryKey: ['dashboard'] }); void client.invalidateQueries({ queryKey: ['facets'] }); void client.invalidateQueries({ queryKey: ['status'] }) }
  const running = status?.running || sync.isPending
  const data = analytics.data
  const custom = query.from || query.to
  const range = custom ? `${query.from || 'Beginning'} → ${query.to || 'Now'}` : query.period === 'all' ? 'All time' : query.period === 'today' || query.period === 'yesterday' ? query.period : `This ${query.period}`
  return <div className="app-shell">
    <a href="#dashboard" className="skip-link">Skip to dashboard</a>
    <header className="app-header">
      <div className="brand"><span className="brand-mark"><ChartNoAxesCombined size="1.25em" /></span><span>Token<span className="brand-light">Insights</span></span><span className="local-badge">LOCAL</span></div>
      <div className="header-actions"><span className="host"><span className="status-dot" />{bootstrap.hostname}</span><span className="header-divider" />
        <button className="icon-button" title={`Theme: ${theme}`} aria-label={`Theme: ${theme}. Change theme`} onClick={() => dispatch({ type: 'theme', value: theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system' })}>{theme === 'dark' ? <Moon size="1.1em" /> : theme === 'light' ? <Sun size="1.1em" /> : <Monitor size="1.1em" />}</button>
        <button className="button subtle" aria-label="Reload data" title="Reload data" disabled={Boolean(running)} onClick={reload}><RefreshCw size="1em" className={analytics.isFetching && enabled ? 'spin' : ''} /><span>Reload data</span></button>
        <button className="button primary" disabled={Boolean(running)} onClick={() => sync.mutate()}>{running ? <LoaderCircle size="1em" className="spin" /> : <ArrowDownToLine size="1em" />}<span>{running ? 'Syncing…' : 'Sync now'}</span></button>
      </div>
    </header>
    <main id="dashboard" className="dashboard">
      <section className="page-heading"><div><div className="eyebrow page-kicker">YOUR LOCAL USAGE, IN FOCUS</div><h1>Usage overview<span className="heading-dot">.</span></h1><p>A clearer picture of your coding agents.</p></div><div className="heading-status"><span className="range-label">{range}</span><span title={data?.lastSynced ? new Date(data.lastSynced).toISOString() : undefined}>{data ? relativeTime(data.lastSynced) : 'Loading sync history…'} <span className="muted">· {bootstrap.timezone}</span></span></div></section>
      <FilterToolbar facets={facets.data} revision={revision} enabled={enabled} />
      {(statusQuery.error || sync.error) && <ErrorBanner message={statusQuery.error?.message ?? sync.error?.message ?? 'Request failed'} onRetry={() => { if (sync.error) sync.mutate(); else void statusQuery.refetch() }} />}
      {status?.running && <SyncProgress status={status} />}
      {status?.error && <section className="sync-error" role="alert"><CircleAlert size="1.3em" /><div><strong>Sync needs attention</strong><p>{status.error}</p></div><button className="button" onClick={() => sync.mutate()} disabled={Boolean(running)}>Retry sync</button>{!enabled && !recoveryFailed && <button className="button" onClick={() => setInspectedRevision(status.revision)}>Inspect existing data</button>}</section>}
      {enabled && analytics.error && <ErrorBanner message={analytics.error.message} onRetry={reload} />}
      {enabled && facets.error && <ErrorBanner message="Filter values couldn’t load." onRetry={() => void facets.refetch()} />}
      {enabled && !data && !analytics.error && <DashboardSkeleton />}
      {enabled && data && <div className="analytics" aria-busy={analytics.isFetching}>
        {analytics.isPlaceholderData ? <div className="summary-grid" role="status" aria-label="Updating filtered summary">{[1, 2, 3, 4, 5].map(n => <div className="skeleton-card" key={n} />)}</div> : <SummaryCards summary={data.summary} />}
        <div className="view-controls"><nav className="view-tabs" aria-label="Analytics views">{tabs.map(({ id, icon: Icon }) => <button key={id} aria-current={query.tab === id ? 'page' : undefined} onClick={() => dispatch({ type: 'tab', value: id })}><Icon size="1em" />{labels[id]}</button>)}</nav><QuickPeriods /></div>
        {analytics.isPlaceholderData ? <div className="skeleton-chart" role="status" aria-label="Updating filtered results" /> : <><Suspense fallback={<div className="skeleton-chart" role="status" aria-label="Loading chart" />}><UsageChart rows={data.chart} /></Suspense><ResultsTable data={data} /></>}
      </div>}
      <footer className="app-footer"><span><span className="status-dot" />Local data. Your machine.</span><span>OpenCode · Pi · Codex · Claude Code</span></footer>
    </main>
  </div>
}

function SyncProgress({ status }: { status: SyncStatus }) {
  const message = status.phase === 'resetting' ? 'Resetting local usage data for compatibility…'
    : status.phase === 'rebuilding' ? 'Rebuilding usage from all configured local harnesses…'
    : status.phase === 'normalizing' ? 'Normalizing canonical data…' : 'Syncing all supported harnesses…'
  return <section className="sync-progress panel" role="status" aria-live="polite"><div className="panel-heading"><div><h2><LoaderCircle className="spin" size="1em" />Refreshing local usage</h2><p>{message}</p></div></div><div className="sync-harnesses">{Object.entries(status.harnesses).map(([harness, phase]) => <div key={harness} data-phase={phase}><span>{phase === 'synced' || phase === 'skipped' ? <Check size="1em" /> : phase === 'failed' ? <CircleAlert size="1em" /> : <span className={`status-dot ${phase === 'pending' ? 'pending' : ''}`} />}{harness}</span><small>{phase}</small></div>)}</div></section>
}

function ErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="error-banner" role="alert"><CircleAlert size="1.1em" /><span>{message}</span><button className="button" onClick={onRetry}>Retry</button></div>
}

function DashboardSkeleton() {
  return <div className="dashboard-skeleton" role="status" aria-label="Loading dashboard"><div className="summary-grid">{[1, 2, 3, 4, 5].map(n => <div key={n} className="skeleton-card" />)}</div><div className="skeleton-chart" /><span className="sr-only">Loading dashboard…</span></div>
}
