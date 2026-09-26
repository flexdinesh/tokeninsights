import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import './styles.css'

if (import.meta.env.MODE === 'mock') {
  const { worker } = await import('./mocks/browser')
  await worker.start({ onUnhandledRequest: 'bypass' })
}

const client = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 15_000, gcTime: 60_000, refetchOnWindowFocus: true },
  },
})
const root = document.getElementById('root')
if (!root) throw new Error('Application root missing')
createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
