import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  Activity,
  Box,
  ChartNoAxesCombined,
  CircleAlert,
  Command,
  Layers3,
  LoaderCircle,
  GitBranch,
} from 'lucide-react'
import {
  AuthenticationRequired,
  changeBrowserSession,
  useAnalytics,
  useBootstrap,
  useFacets,
} from './api'
import { clearAccountQueries } from './session'
import { allowsDashboardReload, hasCapability, showsCollectorProgress } from './capabilities'
import { TokenLogin } from './components/TokenLogin'
import { CollectorProgress, collectorStages } from './components/CollectorProgress'
import type { Bootstrap, Tab } from './contracts'
import { locationGroupSchema } from './contracts'
import {
  DashboardProvider,
  parseDashboardSearch,
  reduceQuery,
  searchFromQuery,
  useDashboardQuery,
} from './state'
import { useDashboardSync } from './useDashboardSync'
import { labels, serverTimeZoneLabel } from './format'
import { FilterToolbar, QuickPeriods } from './components/Filters'
import { DashboardHeader } from './components/DashboardHeader'
import { DashboardResults } from './components/DashboardResults'
import { Button } from './components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './components/ui/select'

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
  const client = useQueryClient()
  const navigate = useNavigate({ from: '/$tab' })
  const [authenticationRequired, setAuthenticationRequired] = useState(false)
  const [sessionChanging, setSessionChanging] = useState(false)
  const [sessionError, setSessionError] = useState('')
  const bootstrap = useBootstrap(!authenticationRequired)
  useEffect(
    () =>
      client.getQueryCache().subscribe((event) => {
        if (
          event.type !== 'updated' ||
          !(event.query.state.error instanceof AuthenticationRequired)
        )
          return
        setAuthenticationRequired(true)
        void clearAccountQueries(client)
      }),
    [client],
  )

  async function signIn() {
    await clearAccountQueries(client)
    await navigate({
      to: '/$tab',
      params: { tab: 'tokens' },
      search: parseDashboardSearch({}),
      replace: true,
    })
    setSessionError('')
    setAuthenticationRequired(false)
  }

  async function signOut() {
    setAuthenticationRequired(true)
    setSessionChanging(true)
    setSessionError('')
    try {
      await clearAccountQueries(client)
      await navigate({
        to: '/$tab',
        params: { tab: 'tokens' },
        search: parseDashboardSearch({}),
        replace: true,
      })
      await changeBrowserSession()
    } catch {
      setSessionError('Couldn’t end the server session. Sign in again to replace it.')
    } finally {
      setSessionChanging(false)
    }
  }

  if (authenticationRequired || bootstrap.error instanceof AuthenticationRequired) {
    return <TokenLogin onSignIn={signIn} disabled={sessionChanging} sessionError={sessionError} />
  }
  const data = bootstrap.data
  if (!data) return <ConnectionScreen error={bootstrap.error} retry={bootstrap.refetch} />
  if (!hasCapability(data, 'web-dashboard') || !data.permissions.includes('read')) {
    return (
      <ConnectionScreen
        error={new Error('This server does not support the web dashboard.')}
        retry={bootstrap.refetch}
      />
    )
  }
  return (
    <DashboardProvider key={`${data.instanceId}/${data.datasetId}`} defaults={data.defaults}>
      <DashboardShell
        bootstrap={data}
        connectionError={bootstrap.error}
        retryConnection={bootstrap.refetch}
        onSignOut={signOut}
      />
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
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <img src="/tokeninsights-logo.png" alt="" className="brand-mark" />
          <span>
            Token<span className="brand-light">Insights</span>
          </span>
        </div>
      </header>
      <main className="startup-state">
        {error ? (
          <>
            <CircleAlert />
            <h1>Couldn’t connect to {window.location.host}</h1>
            <p>{error.message}</p>
            <Button onClick={() => void retry()}>Retry Connection</Button>
          </>
        ) : (
          <>
            <LoaderCircle className="spin" />
            <p>Connecting to {window.location.host}…</p>
          </>
        )}
      </main>
    </div>
  )
}

function DashboardShell({
  bootstrap,
  connectionError,
  retryConnection,
  onSignOut,
}: {
  bootstrap: Bootstrap
  connectionError: Error | null
  retryConnection: () => Promise<unknown>
  onSignOut: () => Promise<void>
}) {
  const { query, setLocationGroup } = useDashboardQuery()
  const showProgress = showsCollectorProgress(bootstrap)
  const controller = useDashboardSync(bootstrap.datasetId, {
    enabled: showProgress,
    instanceId: bootstrap.instanceId,
  })
  const { revision, analyticsEnabled: enabled } = controller
  const analytics = useAnalytics(
    query,
    revision,
    enabled && hasCapability(bootstrap, 'usage'),
    controller.identity,
  )
  const facets = useFacets(
    query,
    revision,
    enabled && hasCapability(bootstrap, 'facets'),
    controller.identity,
  )
  const statusLabel = dashboardStatusLabel(
    controller,
    analytics,
    facets,
    bootstrap,
    connectionError,
  )
  return (
    <div className="app-shell">
      <a href="#dashboard" className="skip-link">
        Skip to dashboard
      </a>
      <DashboardHeader
        hostname={bootstrap.hostname}
        timezone={bootstrap.timezone}
        reloading={controller.reloading}
        statusLabel={statusLabel}
        localMachine={bootstrap.serverKind === 'personal'}
        allowReload={allowsDashboardReload(bootstrap)}
        lastSynced={analytics.data?.dashboard.lastSynced}
        onReload={() => void controller.reload()}
      />
      <main id="dashboard" className="dashboard">
        <h1 className="sr-only">Token usage</h1>
        {bootstrap.serverKind === 'hosted' && (
          <Button variant="ghost" onClick={() => void onSignOut()}>
            Sign out
          </Button>
        )}
        {showProgress && <CollectorProgress progress={controller.progressQuery.data} />}
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
                  if (parsed.success) setLocationGroup(parsed.data)
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
          {hasCapability(bootstrap, 'facets') && (
            <FilterToolbar
              facets={facets.data}
              revision={revision}
              enabled={enabled}
              identity={controller.identity}
            />
          )}
          <DashboardResults
            analytics={analytics}
            facets={facets}
            controller={controller}
            timezone={bootstrap.timezone}
            connectionError={connectionError}
            retryConnection={retryConnection}
          />
        </div>
        <footer className="app-footer">
          <span>
            <span className="status-dot" />
            {bootstrap.hostname} · {window.location.origin} ·{' '}
            {serverTimeZoneLabel(bootstrap.timezone)}
          </span>
          <span>OpenCode · Pi · Codex · Claude Code</span>
        </footer>
      </main>
    </div>
  )
}

function dashboardStatusLabel(
  controller: ReturnType<typeof useDashboardSync>,
  analytics: ReturnType<typeof useAnalytics>,
  facets: ReturnType<typeof useFacets>,
  bootstrap: Bootstrap,
  connectionError: Error | null,
): string {
  if (connectionError || controller.statusQuery.error) return 'Connection unavailable'
  if (analytics.error) return 'Usage couldn’t load'
  if (facets.error) return 'Filters couldn’t load'
  if (showsCollectorProgress(bootstrap) && controller.progressQuery.error)
    return 'Collection status unavailable'
  if (controller.collectionFailed) return 'Collection needs attention'
  if (controller.collectionStage) return collectorStages[controller.collectionStage]
  if (controller.processingPending)
    return controller.processingBackoff ? 'Processing retry pending' : 'Processing usage'
  if (controller.loading || controller.reloading) return 'Loading…'
  if (
    controller.analyticsEnabled &&
    hasCapability(bootstrap, 'usage') &&
    (analytics.isPending || analytics.isFetching)
  )
    return analytics.data ? 'Refreshing usage…' : 'Loading…'
  if (controller.analyticsEnabled && hasCapability(bootstrap, 'facets') && facets.isPending)
    return 'Loading filters…'
  return 'Ready'
}
