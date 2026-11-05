import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { DashboardClientProvider, router } from '@tokeninsights/dashboard'
import { httpDashboardClient } from './http-client'
import { BrowserViewer } from './viewer'
import '@tokeninsights/dashboard/styles.css'

if (import.meta.env.MODE === 'mock') {
  const { worker } = await import('./mocks/browser')
  await worker.start({ onUnhandledFrame: 'bypass' })
}

const client = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 15_000,
      gcTime: 60_000,
      refetchOnWindowFocus: true,
    },
  },
})
const root = document.getElementById('root')
if (!root) throw new Error('Application root missing')
// The design catalog renders outside the app router because the root route
// requires a server bootstrap. Production builds drop this branch and chunk.
if (import.meta.env.DEV && window.location.pathname === '/_design') {
  const { DesignCatalog } = await import('../../dashboard/src/design/DesignCatalog')
  createRoot(root).render(
    <StrictMode>
      <DesignCatalog />
    </StrictMode>,
  )
} else {
  createRoot(root).render(
    <StrictMode>
      <DashboardClientProvider client={httpDashboardClient}>
        <QueryClientProvider client={client}>
          <BrowserViewer />
          <RouterProvider router={router} />
        </QueryClientProvider>
      </DashboardClientProvider>
    </StrictMode>,
  )
}
