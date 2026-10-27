import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { createAppRouter } from './router'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'

const clients: QueryClient[] = []

it('custom dates validate order before applying and disappear when returning to a preset', async () => {
  const user = userEvent.setup()
  const { history } = mount()
  await user.click(await screen.findByRole('button', { name: /^Date range:/ }))
  expect(screen.queryByLabelText('From date')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Custom' }))
  fireEvent.change(screen.getByLabelText('From date'), { target: { value: '2026-10-02' } })
  fireEvent.change(screen.getByLabelText('To date'), { target: { value: '2026-10-01' } })
  expect(screen.getByRole('button', { name: 'Apply range' })).toBeDisabled()
  expect(screen.getByRole('alert')).toHaveTextContent('From must not be after to.')
  fireEvent.change(screen.getByLabelText('To date'), { target: { value: '2026-10-03' } })
  await user.click(screen.getByRole('button', { name: 'Apply range' }))
  await waitFor(() => expect(history.location.search).toContain('from=2026-10-02'))
  await user.click(screen.getByRole('button', { name: /^Date range:/ }))
  expect(screen.getByLabelText('From date')).toHaveValue('2026-10-02')
  await user.click(screen.getByRole('button', { name: 'All time' }))
  await user.click(screen.getByRole('button', { name: /^Date range:/ }))
  expect(screen.queryByLabelText('From date')).not.toBeInTheDocument()
})
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  vi.unstubAllGlobals()
  window.localStorage.clear()
})

function mount(path = '/tokens', options = { confirmed: true, excluded: true, failing: false }) {
  const history = createMemoryHistory({ initialEntries: [path] })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  const requests: URL[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(async (input) => {
      const url = new URL(
        typeof input === 'string' ? input : input instanceof URL ? input.href : input.url,
        'http://localhost',
      )
      requests.push(url)
      let body: unknown
      if (url.pathname.endsWith('/instance')) body = mockBootstrap
      else if (url.pathname.endsWith('/status')) body = mockSyncStatus(1)
      else if (url.pathname.endsWith('/facets')) body = mockFacets
      else {
        const estimated = url.searchParams.get('quality') === 'estimated'
        if (estimated && options.failing)
          return new Response(JSON.stringify({ code: 'unavailable', message: 'Unavailable' }), {
            status: 503,
          })
        const data = mockDashboard(url.searchParams.get('tab'))
        const present = estimated ? options.excluded : options.confirmed
        body = {
          ...data,
          quality: estimated ? 'estimated' : 'confirmed',
          rows: present ? data.rows : [],
          chart: present ? data.chart : [],
          rowCount: present ? data.rowCount : 0,
          summary: present
            ? { ...data.summary, total: estimated ? 120 : data.summary.total }
            : {
                total: 0,
                input: 0,
                output: 0,
                cacheRead: 0,
                cacheWrite: 0,
                reasoning: 0,
                sessions: 0,
                syncedSessions: 0,
              },
        }
      }
      return new Response(JSON.stringify(body))
    }),
  )
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={createAppRouter(history)} />
    </QueryClientProvider>,
  )
  return { history, requests, options, client }
}

it('excluded review preserves location filters and history without merging confirmed totals', async () => {
  const user = userEvent.setup()
  const { history, requests } = mount(
    '/repo?period=all&locationGroup=directory&directory=directory-1',
  )
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  await user.click(await screen.findByRole('button', { name: 'Review excluded usage' }))
  expect(await screen.findByLabelText('Excluded tokens: 120')).toBeVisible()
  expect(screen.queryByLabelText('Total tokens: 647,000')).not.toBeInTheDocument()
  expect(
    requests
      .filter((url) => url.searchParams.get('quality') === 'estimated')
      .every((url) => url.searchParams.get('directory') === 'directory-1'),
  ).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Back to usage' }))
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(history.location.search).toContain('directory=directory-1')
  history.back()
  expect(await screen.findByLabelText('Excluded tokens: 120')).toBeVisible()
})

it('no matching excluded evidence omits review controls', async () => {
  const { client } = mount('/models?model=gpt-5', {
    confirmed: true,
    excluded: false,
    failing: false,
  })
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  await waitFor(() => expect(client.isFetching()).toBe(0))
  expect(screen.queryByRole('button', { name: 'Review excluded usage' })).not.toBeInTheDocument()
})

it('excluded evidence alone offers review instead of claiming nothing is saved', async () => {
  const user = userEvent.setup()
  mount('/tokens', { confirmed: false, excluded: true, failing: false })
  expect(await screen.findByText('No usage included in your total yet.')).toBeVisible()
  expect(
    screen.queryByText('No usage saved yet. Restart tokeninsights web to collect usage.'),
  ).not.toBeInTheDocument()
  const [review] = await screen.findAllByRole('button', { name: 'Review excluded usage' })
  if (!review) throw new Error('Expected excluded review recovery')
  await user.click(review)
  expect(await screen.findByLabelText('Excluded tokens: 120')).toBeVisible()
})

it('excluded summary failure retries independently while confirmed usage stays visible', async () => {
  const user = userEvent.setup()
  const { options } = mount('/tokens', { confirmed: true, excluded: true, failing: true })
  expect(await screen.findByText('Excluded usage couldn’t load.')).toBeVisible()
  expect(screen.getByLabelText('Total tokens: 647,000')).toBeVisible()
  options.failing = false
  await user.click(screen.getByRole('button', { name: 'Retry excluded usage' }))
  expect(await screen.findByRole('button', { name: 'Review excluded usage' })).toBeVisible()
})

it('column preferences and filter drafts survive view navigation and history', async () => {
  const user = userEvent.setup()
  const { history } = mount()
  await user.click(await screen.findByRole('button', { name: 'Columns' }))
  await user.click(screen.getByRole('checkbox', { name: 'Output' }))
  await user.keyboard('{Escape}')
  await user.click(
    within(screen.getByRole('region', { name: 'Dashboard filters' })).getByRole('button', {
      name: 'Model',
    }),
  )
  await user.type(screen.getByRole('textbox', { name: 'Search model' }), 'gpt')
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('link', { name: 'Models' }))
  expect(await screen.findByRole('region', { name: 'Models details' })).toBeVisible()
  expect(screen.queryByRole('columnheader', { name: 'Output' })).not.toBeInTheDocument()
  await user.click(
    within(screen.getByRole('region', { name: 'Dashboard filters' })).getByRole('button', {
      name: 'Model',
    }),
  )
  expect(screen.getByRole('textbox', { name: 'Search model' })).toHaveValue('gpt')
  await user.keyboard('{Escape}')
  history.back()
  expect(await screen.findByRole('region', { name: 'Tokens details' })).toBeVisible()
  expect(screen.queryByRole('columnheader', { name: 'Output' })).not.toBeInTheDocument()
})
