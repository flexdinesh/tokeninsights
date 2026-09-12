import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SourceSelector } from './SourceSelector'
import { SourceProvider } from '../source-context'
import { createSourceStore } from '../sources'
import type { Bootstrap } from '../contracts'

const instance: Bootstrap = {
  apiVersion: 'v1',
  serverVersion: '1.2.3',
  hostname: 'remote-workstation',
  timezone: 'Australia/Sydney',
  capabilities: ['usage', 'facets', 'sync'],
  defaults: {
    period: 'month',
    bucket: 'day',
    from: '',
    to: '',
    providers: [],
    models: [],
    harnesses: [],
    sessions: [],
  },
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function renderSelector() {
  const store = createSourceStore({ origin: 'http://localhost:8765', storage: null })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <SourceProvider store={store}>
        <SourceSelector unavailable={false} />
      </SourceProvider>
    </QueryClientProvider>,
  )
  return { store, client }
}

it('validates a source before saving and selects its server hostname', async () => {
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValue(
      new Response(JSON.stringify(instance), { headers: { 'Content-Type': 'application/json' } }),
    )
  vi.stubGlobal('fetch', fetcher)
  const { store, client } = renderSelector()
  const user = userEvent.setup()

  await user.click(screen.getByRole('button', { name: 'Choose data source' }))
  await user.type(screen.getByLabelText('Add source'), '10.0.0.2:8765')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  await waitFor(() => expect(store.getSnapshot().activeUrl).toBe('http://10.0.0.2:8765'))
  expect(fetcher).toHaveBeenCalledWith(
    'http://10.0.0.2:8765/api/v1/instance',
    expect.objectContaining({ method: 'GET' }),
  )
  expect(screen.getByRole('button', { name: 'Choose data source' })).toHaveTextContent(
    'remote-workstation',
  )
  client.clear()
})

it('rejects an incompatible source without persisting it', async () => {
  const incompatible: Bootstrap = { ...instance, capabilities: ['usage'] }
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify(incompatible), {
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
  const { store, client } = renderSelector()
  const user = userEvent.setup()

  await user.click(screen.getByRole('button', { name: 'Choose data source' }))
  await user.type(screen.getByLabelText('Add source'), 'remote:8765')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Source lacks required capabilities: facets, sync',
  )
  expect(store.getSnapshot().sources).toHaveLength(1)
  client.clear()
})

it('rejects an unreachable source without persisting it', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockRejectedValue(new Error('Network offline')))
  const { store, client } = renderSelector()
  const user = userEvent.setup()

  await user.click(screen.getByRole('button', { name: 'Choose data source' }))
  await user.type(screen.getByLabelText('Add source'), 'offline:8765')
  await user.click(screen.getByRole('button', { name: 'Add' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('Network offline')
  expect(store.getSnapshot().sources).toHaveLength(1)
  client.clear()
})
