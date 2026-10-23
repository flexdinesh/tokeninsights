import {
  createRootRoute,
  createRoute,
  createMemoryHistory,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { App, DashboardView } from './App'
import { SettingsPage } from './components/SettingsPage'
import { tabSchema } from './contracts'
import { parseDashboardSearch, parseDashboardSearchParams, stringifyDashboardSearch } from './query'
import { Button, Heading, StartupShell } from '@/ui'

function NotFound() {
  return (
    <StartupShell>
      <p>Dashboard route not found.</p>
    </StartupShell>
  )
}

function DashboardError() {
  return (
    <StartupShell role="alert">
      <Heading as="h1" variant="page">
        Dashboard couldn’t load.
      </Heading>
      <Button onClick={() => window.location.reload()}>Reload page</Button>
    </StartupShell>
  )
}

const rootRoute = createRootRoute({
  component: App,
  errorComponent: DashboardError,
  notFoundComponent: NotFound,
  validateSearch: parseDashboardSearch,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: ({ search }) => {
    throw redirect({
      to: '/$tab',
      params: { tab: 'tokens' },
      search,
      replace: true,
    })
  },
})

export const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '$tab',
  params: {
    parse: ({ tab }) => {
      const parsed = tabSchema.safeParse(tab)
      return parsed.success ? { tab: parsed.data } : false
    },
    stringify: ({ tab }) => ({ tab }),
  },
  component: DashboardView,
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: 'settings',
  component: SettingsPage,
})
const routeTree = rootRoute.addChildren([indexRoute, dashboardRoute, settingsRoute])

export function createAppRouter(history?: ReturnType<typeof createMemoryHistory>) {
  return createRouter({
    routeTree,
    history,
    defaultPreload: 'intent',
    parseSearch: parseDashboardSearchParams,
    stringifySearch: stringifyDashboardSearch,
  })
}

export const router = createAppRouter()

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
