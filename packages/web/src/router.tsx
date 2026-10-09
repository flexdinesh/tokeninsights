import {
  Outlet,
  createRootRoute,
  createRoute,
  createMemoryHistory,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { App } from './App'
import { tabSchema } from './contracts'
import { parseDashboardSearch, parseDashboardSearchParams, stringifyDashboardSearch } from './state'
import { Button } from './components/ui/button'

function Root() {
  return <Outlet />
}

function NotFound() {
  return <main className="startup-state">Dashboard route not found.</main>
}

function DashboardError() {
  return (
    <main className="startup-state" role="alert">
      <h1>Dashboard couldn’t load.</h1>
      <Button onClick={() => window.location.reload()}>Reload page</Button>
    </main>
  )
}

const rootRoute = createRootRoute({
  component: Root,
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
  component: App,
})

const routeTree = rootRoute.addChildren([indexRoute, dashboardRoute])

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
