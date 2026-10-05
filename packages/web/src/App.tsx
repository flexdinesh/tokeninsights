import { Link } from '@tanstack/react-router'
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
import { useAnalytics, useBootstrap, useFacets } from './api'
import type { Bootstrap, Tab } from './contracts'
import { locationGroupSchema } from './contracts'
import { DashboardProvider, reduceQuery, searchFromQuery, useDashboardQuery } from './state'
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
  const bootstrap = useBootstrap()
  const data = bootstrap.data
  if (!data) return <ConnectionScreen error={bootstrap.error} retry={bootstrap.refetch} />
  return (
    <DashboardProvider defaults={data.defaults}>
      <DashboardShell
        bootstrap={data}
        connectionError={bootstrap.error}
        retryConnection={bootstrap.refetch}
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
}: {
  bootstrap: Bootstrap
  connectionError: Error | null
  retryConnection: () => Promise<unknown>
}) {
  const { query, setLocationGroup } = useDashboardQuery()
  const controller = useDashboardSync()
  const { revision, analyticsEnabled: enabled } = controller
  const analytics = useAnalytics(query, revision, enabled, false, controller.identity)
  const facets = useFacets(query, revision, enabled, '', controller.identity)
  const serverUnavailable = Boolean(
    connectionError || controller.statusQuery.error || analytics.error || facets.error,
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
        serverUnavailable={serverUnavailable}
        lastSynced={analytics.data?.dashboard.lastSynced}
        onReload={() => void controller.reload()}
      />
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
          <FilterToolbar
            facets={facets.data}
            revision={revision}
            enabled={enabled}
            identity={controller.identity}
          />
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
