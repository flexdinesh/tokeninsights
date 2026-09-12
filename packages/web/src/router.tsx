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
import type { DashboardSearch } from './state'

function Root() {
  return <Outlet />
}

function NotFound() {
  return <main className="startup-state">Dashboard route not found.</main>
}

const rootRoute = createRootRoute({
  component: Root,
  notFoundComponent: NotFound,
  validateSearch: parseDashboardSearch,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: ({ search }) => {
    const cleanSearch: DashboardSearch = { ...search, legacyTab: undefined }
    throw redirect({
      to: '/$tab',
      params: { tab: search.legacyTab ?? 'tokens' },
      search: cleanSearch,
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
