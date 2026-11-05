import { useCallback, useMemo } from 'react'
import { Outlet } from '@tanstack/react-router'
import { CircleAlert } from 'lucide-react'
import { useAnalytics, useBootstrap, useConnection, useFacets, useRefresh } from './api'
import { hasCapability, showsCollectorProgress } from './capabilities'
import { IdentityContext, useIdentity } from './identity'
import { collectorStages } from './components/CollectorProgress'
import type { DashboardStatusValue } from './components/DashboardStatus'
import type { Bootstrap, ViewerConnection } from '@tokeninsights/contracts'
import { locationGroupSchema } from '@tokeninsights/contracts'
import { DashboardProvider, useDashboardQuery } from './state'
import { useDashboardSync } from './useDashboardSync'
import { serverTimeZoneLabel } from './format'
import { FilterToolbar } from './components/Filters'
import { DateNavigation } from './components/DateNavigation'
import { DashboardHeader } from './components/DashboardHeader'
import { DashboardSidebar } from './components/DashboardSidebar'
import { DashboardResults } from './components/DashboardResults'
import {
  AppShell,
  Button,
  ControlLabel,
  Heading,
  PageContent,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  StartupShell,
  StatusDot,
} from './ui'

export function App() {
  const bootstrap = useBootstrap()
  const connection = useConnection()
  const retryConnection = useCallback(async () => {
    await Promise.all([bootstrap.refetch(), connection.refetch()])
  }, [bootstrap.refetch, connection.refetch])
  const identity = useMemo(
    () =>
      bootstrap.data
        ? {
            bootstrap: bootstrap.data,
            connection: connection.data,
            connectionError: bootstrap.error ?? connection.error,
            retryConnection,
          }
        : undefined,
    [bootstrap.data, bootstrap.error, connection.data, connection.error, retryConnection],
  )
  const data = bootstrap.data
  if (!data)
    return (
      <ConnectionScreen
        connection={connection.data}
        error={bootstrap.error}
        retry={retryConnection}
      />
    )
  if (!hasCapability(data, 'web-dashboard') || !data.permissions.includes('read')) {
    return (
      <ConnectionScreen
        error={new Error('This server does not support the web dashboard.')}
        connection={connection.data}
        retry={retryConnection}
      />
    )
  }
  return (
    <IdentityContext.Provider value={identity}>
      <Outlet />
    </IdentityContext.Provider>
  )
}

export function DashboardView() {
  const identity = useIdentity()
  return (
    <DashboardProvider
      key={`${identity.bootstrap.instanceId}/${identity.bootstrap.datasetId}`}
      defaults={identity.bootstrap.defaults}
    >
      <DashboardShell {...identity} />
    </DashboardProvider>
  )
}

function ConnectionScreen({
  connection,
  error,
  retry,
}: {
  connection: ViewerConnection | undefined
  error: Error | null
  retry: () => Promise<unknown>
}) {
  const target = connection?.target ?? 'requested backend'
  return (
    <StartupShell>
      {connection && (
        <p role="status">
          {connection.transport.toUpperCase()} · {target} · {error ? 'unavailable' : 'connecting'}
        </p>
      )}
      {error ? (
        <>
          <CircleAlert aria-hidden="true" />
          <Heading as="h1" variant="page">
            Couldn’t connect to {target}
          </Heading>
          <p>{error.message}</p>
          <Button onClick={() => void retry()}>Retry Connection</Button>
        </>
      ) : (
        <>
          <Spinner />
          <p>Connecting to {target}…</p>
        </>
      )}
    </StartupShell>
  )
}

function DashboardShell({
  bootstrap,
  connectionError,
  retryConnection,
  connection,
}: {
  bootstrap: Bootstrap
  connectionError: Error | null
  retryConnection: () => Promise<unknown>
  connection: ViewerConnection | undefined
}) {
  const refresh = useRefresh()
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
  const excludedUsage = useAnalytics(
    {
      ...query,
      quality: 'estimated',
      tab: query.tab === 'repo' ? 'repo' : 'tokens',
      bucket: 'day',
      sort: query.tab === 'repo' ? 'total' : 'date',
      direction: 'desc',
      page: 1,
      pageSize: 1,
    },
    revision,
    enabled && analytics.isSuccess && query.quality === 'confirmed',
    controller.identity,
  )
  const facets = useFacets(
    query,
    revision,
    enabled && hasCapability(bootstrap, 'facets'),
    controller.identity,
  )
  const status = dashboardStatus(controller, analytics, facets, bootstrap, connectionError, refresh)
  return (
    <AppShell
      skipTarget="dashboard"
      sidebar={<DashboardSidebar query={query} />}
      header={
        <DashboardHeader
          hostname={bootstrap.hostname}
          timezone={bootstrap.timezone}
          status={status}
          progress={showProgress ? controller.progressQuery.data : undefined}
          processUsage={
            controller.statusQuery.error ? undefined : controller.statusQuery.data?.processUsage
          }
          connection={connection}
          connectionUnavailable={Boolean(connectionError || controller.statusQuery.error)}
          instanceId={bootstrap.instanceId}
          refreshPending={refresh.isPending || controller.collectionPending}
          onRefresh={() => refresh.mutate()}
          lastSynced={analytics.data?.dashboard.lastSynced}
        />
      }
    >
      <PageContent id="dashboard">
        <div className="mb-2 flex flex-wrap items-start gap-x-3 gap-y-2">
          {hasCapability(bootstrap, 'facets') && (
            <FilterToolbar
              facets={facets.data}
              revision={revision}
              enabled={enabled}
              identity={controller.identity}
            />
          )}
          <DateNavigation timezone={bootstrap.timezone} />
        </div>
        {query.tab === 'repo' && (
          <div className="flex flex-wrap gap-3 pb-3" role="group" aria-label="Repo view options">
            <ControlLabel label="Group by">
              <Select
                value={query.locationGroup}
                onValueChange={(value) => {
                  const parsed = locationGroupSchema.safeParse(value)
                  if (parsed.success) setLocationGroup(parsed.data)
                }}
              >
                <SelectTrigger size="compact" aria-label="Group by location">
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
            </ControlLabel>
          </div>
        )}
        <div>
          <DashboardResults
            analytics={analytics}
            excludedUsage={excludedUsage}
            facets={facets}
            controller={controller}
            refreshError={refresh.error}
            timezone={bootstrap.timezone}
            connectionError={connectionError}
            retryConnection={retryConnection}
          />
        </div>
        <footer className="flex justify-between gap-3 pt-3 text-label text-ink-subtle max-compact:flex-col max-compact:gap-2 [&>span]:min-w-0 [&>span]:wrap-anywhere">
          <span className="flex items-center gap-2">
            <StatusDot
              tone={
                status.state === 'attention'
                  ? 'danger'
                  : status.state === 'loading'
                    ? 'pending'
                    : 'success'
              }
            />
            {bootstrap.hostname} · {connection?.target ?? 'Connection details unavailable'} ·{' '}
            {serverTimeZoneLabel(bootstrap.timezone)}
          </span>
          <span>OpenCode · Pi · Codex · Claude Code</span>
        </footer>
      </PageContent>
    </AppShell>
  )
}

function dashboardStatus(
  controller: ReturnType<typeof useDashboardSync>,
  analytics: ReturnType<typeof useAnalytics>,
  facets: ReturnType<typeof useFacets>,
  bootstrap: Bootstrap,
  connectionError: Error | null,
  refresh: ReturnType<typeof useRefresh>,
): DashboardStatusValue {
  if (connectionError || controller.statusQuery.error)
    return { label: 'Connection unavailable', state: 'attention' }
  if (refresh.error) return { label: 'Refresh failed', state: 'attention' }
  if (refresh.isPending) return { label: 'Refreshing usage…', state: 'loading' }
  if (analytics.error) return { label: 'Usage couldn’t load', state: 'attention' }
  if (facets.error) return { label: 'Filters couldn’t load', state: 'attention' }
  if (showsCollectorProgress(bootstrap) && controller.progressQuery.error)
    return { label: 'Collection status unavailable', state: 'attention' }
  if (controller.collectionFailed)
    return { label: 'Collection needs attention', state: 'attention' }
  if (controller.collectionStage)
    return { label: collectorStages[controller.collectionStage], state: 'loading' }
  if (controller.processingPending)
    return {
      label: controller.processingBackoff ? 'Processing retry pending' : 'Processing usage',
      state: controller.processingBackoff ? 'attention' : 'loading',
    }
  if (controller.loading) return { label: 'Loading…', state: 'loading' }
  if (
    controller.analyticsEnabled &&
    hasCapability(bootstrap, 'usage') &&
    (analytics.isPending || analytics.isFetching)
  )
    return { label: analytics.data ? 'Refreshing usage…' : 'Loading…', state: 'loading' }
  if (controller.analyticsEnabled && hasCapability(bootstrap, 'facets') && facets.isPending)
    return { label: 'Loading filters…', state: 'loading' }
  return { label: 'Ready', state: 'ready' }
}
