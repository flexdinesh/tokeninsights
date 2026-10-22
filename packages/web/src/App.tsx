import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Outlet, useNavigate } from '@tanstack/react-router'
import { CircleAlert, LoaderCircle } from 'lucide-react'
import { changeBrowserSession, useAnalytics, useBootstrap, useFacets } from './api'
import { AuthenticationRequired } from './http'
import { clearAccountQueries } from './session'
import { hasCapability, showsCollectorProgress } from './capabilities'
import { PasswordLogin } from './components/PasswordLogin'
import { useAuthInfo } from './account'
import { IdentityContext, useIdentity } from './identity'
import { collectorStages } from './components/CollectorProgress'
import type { DashboardStatusValue } from './components/DashboardStatus'
import type { Bootstrap } from './contracts'
import { locationGroupSchema } from './contracts'
import { parseDashboardSearch } from './query'
import { DashboardProvider, useDashboardQuery } from './state'
import { useDashboardSync } from './useDashboardSync'
import { serverTimeZoneLabel } from './format'
import { FilterToolbar } from './components/Filters'
import { DateNavigation } from './components/DateNavigation'
import { DashboardHeader } from './components/DashboardHeader'
import { DashboardSidebar } from './components/DashboardSidebar'
import { Brand } from './components/Brand'
import { DashboardResults } from './components/DashboardResults'
import { Button } from './components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './components/ui/select'

export function App() {
  const client = useQueryClient()
  const navigate = useNavigate()
  const [authenticationRequired, setAuthenticationRequired] = useState(false)
  const [sessionChanging, setSessionChanging] = useState(false)
  const [sessionError, setSessionError] = useState('')
  const bootstrap = useBootstrap(!authenticationRequired)
  const needsAuthentication =
    authenticationRequired || bootstrap.error instanceof AuthenticationRequired
  const authInfo = useAuthInfo(needsAuthentication && !sessionChanging)
  useEffect(
    () =>
      client.getQueryCache().subscribe((event) => {
        if (
          event.type !== 'updated' ||
          !(event.query.state.error instanceof AuthenticationRequired)
        )
          return
        setAuthenticationRequired(true)
        setSessionChanging(true)
        void clearAccountQueries(client).finally(() => setSessionChanging(false))
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

  const signOut = useCallback(async () => {
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
  }, [client, navigate])

  const sessionEnded = useCallback(
    async (message = '') => {
      setAuthenticationRequired(true)
      setSessionChanging(true)
      await clearAccountQueries(client)
      setSessionError(message)
      setSessionChanging(false)
    },
    [client],
  )

  const identity = useMemo(
    () =>
      bootstrap.data
        ? {
            bootstrap: bootstrap.data,
            connectionError: bootstrap.error,
            retryConnection: bootstrap.refetch,
            onSignOut: signOut,
            onSessionEnded: sessionEnded,
          }
        : undefined,
    [bootstrap.data, bootstrap.error, bootstrap.refetch, signOut, sessionEnded],
  )

  if (needsAuthentication) {
    if (!authInfo.data) return <ConnectionScreen error={authInfo.error} retry={authInfo.refetch} />
    if (authInfo.data.serverKind !== 'hosted' || !authInfo.data.loginMethods.includes('password'))
      return (
        <ConnectionScreen
          error={new Error('This server does not support password sign in.')}
          retry={authInfo.refetch}
        />
      )
    return (
      <PasswordLogin
        onSignIn={signIn}
        signupEnabled={authInfo.data.signupEnabled}
        disabled={sessionChanging}
        sessionError={sessionError}
      />
    )
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
  error,
  retry,
}: {
  error: Error | null
  retry: () => Promise<unknown>
}) {
  return (
    <div className="app-shell">
      <header className="app-header">
        <Brand />
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
  const status = dashboardStatus(controller, analytics, facets, bootstrap, connectionError)
  return (
    <div className="app-shell dashboard-shell">
      <a href="#dashboard" className="skip-link">
        Skip to dashboard
      </a>
      <DashboardSidebar
        query={query}
        localMachine={bootstrap.serverKind === 'personal'}
        onSignOut={bootstrap.serverKind === 'hosted' ? onSignOut : undefined}
      />
      <div className="dashboard-main">
        <DashboardHeader
          hostname={bootstrap.hostname}
          timezone={bootstrap.timezone}
          status={status}
          progress={showProgress ? controller.progressQuery.data : undefined}
          processUsage={
            controller.statusQuery.error ? undefined : controller.statusQuery.data?.processUsage
          }
          localMachine={bootstrap.serverKind === 'personal'}
          lastSynced={analytics.data?.dashboard.lastSynced}
        />
        <main id="dashboard" className="dashboard">
          <div className="dashboard-toolbar">
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
            <DashboardResults
              localMachine={bootstrap.serverKind === 'personal'}
              analytics={analytics}
              excludedUsage={excludedUsage}
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
    </div>
  )
}

function dashboardStatus(
  controller: ReturnType<typeof useDashboardSync>,
  analytics: ReturnType<typeof useAnalytics>,
  facets: ReturnType<typeof useFacets>,
  bootstrap: Bootstrap,
  connectionError: Error | null,
): DashboardStatusValue {
  if (connectionError || controller.statusQuery.error)
    return { label: 'Connection unavailable', state: 'attention' }
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
