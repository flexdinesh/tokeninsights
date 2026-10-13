import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createMemoryHistory } from '@tanstack/react-router'
import { createAppRouter } from './router'
import { mockBootstrap, mockDashboard, mockFacets, mockSyncStatus } from './mocks/data'
import { validPassword } from './account'
import type { APIKeyInfo } from './contracts'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function fixture({
  kind = 'hosted',
  signedIn = true,
  signupEnabled = false,
  path = '/settings',
  rejectMutation,
  rejectRead,
}: {
  kind?: 'personal' | 'hosted'
  signedIn?: boolean
  signupEnabled?: boolean
  path?: string
  rejectMutation?: { path: string; status: number }
  rejectRead?: string
} = {}) {
  let authenticated = signedIn
  let keys: APIKeyInfo[] = []
  const fetcher = vi.fn<typeof fetch>((input, init) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    if (url.endsWith('/auth'))
      return Promise.resolve(
        new Response(
          JSON.stringify({
            serverKind: kind,
            loginMethods: kind === 'hosted' ? ['password'] : [],
            signupEnabled,
          }),
        ),
      )
    if (url.endsWith('/auth/session') || url.endsWith('/auth/signup')) {
      authenticated = init?.method !== 'DELETE'
      return Promise.resolve(new Response(null, { status: 204 }))
    }
    if (!authenticated) return Promise.resolve(new Response(null, { status: 401 }))
    if (rejectRead && url.endsWith(rejectRead) && init?.method === 'GET')
      return Promise.resolve(
        new Response(JSON.stringify({ code: 'dataset-changed', message: 'Dataset changed' }), {
          status: 409,
        }),
      )
    if (rejectMutation && url.endsWith(rejectMutation.path) && init?.method !== 'GET')
      return Promise.resolve(
        new Response(JSON.stringify({ code: 'rejected', message: 'Request rejected' }), {
          status: rejectMutation.status,
        }),
      )
    if (url.endsWith('/account/password')) {
      authenticated = false
      return Promise.resolve(new Response(null, { status: 204 }))
    }
    if (url.endsWith('/account/keys') && init?.method === 'POST') {
      keys = [
        {
          tokenId: 'key-1',
          label: 'Work laptop',
          permissions: ['read', 'ingest'],
          createdAt: '2026-10-13T00:00:00Z',
        },
      ]
      return Promise.resolve(
        new Response(
          JSON.stringify({
            tokenId: 'key-1',
            token: 'one-time-secret',
            permissions: ['read', 'ingest'],
          }),
          { status: 201 },
        ),
      )
    }
    if (url.endsWith('/account/keys/key-1')) {
      keys = keys.map((key) => ({ ...key, revokedAt: '2026-10-13T01:00:00Z' }))
      return Promise.resolve(new Response(null, { status: 204 }))
    }
    const data = url.endsWith('/instance')
      ? { ...mockBootstrap, serverKind: kind }
      : url.endsWith('/account')
        ? {
            userId: 'user',
            datasetId: mockBootstrap.datasetId,
            username: 'alice',
            displayName: 'alice',
            enabled: true,
          }
        : url.endsWith('/account/keys')
          ? { keys }
          : url.endsWith('/status')
            ? mockSyncStatus(1)
            : url.includes('/facets')
              ? mockFacets
              : mockDashboard('tokens')
    return Promise.resolve(new Response(JSON.stringify(data)))
  })
  vi.stubGlobal('fetch', fetcher)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter(createMemoryHistory({ initialEntries: [path] }))
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { client, fetcher }
}

it('local settings stay unavailable without requesting account endpoints', async () => {
  const { client, fetcher } = fixture({ kind: 'personal' })
  expect(await screen.findByRole('heading', { name: 'Account settings unavailable' })).toBeVisible()
  expect(
    fetcher.mock.calls.every(([input]) => typeof input === 'string' && !input.includes('/account')),
  ).toBe(true)
  await userEvent.click(screen.getByRole('link', { name: 'Return to usage' }))
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(screen.queryByRole('link', { name: 'Account settings' })).not.toBeInTheDocument()
  client.clear()
})

it('creates a named sync key, reveals it once, and revokes it without ending the session', async () => {
  const { client, fetcher } = fixture()
  const keyName = await screen.findByLabelText('Key name')
  expect(keyName).not.toHaveFocus()
  fireEvent.change(keyName, { target: { value: 'Work laptop' } })
  await userEvent.click(screen.getByRole('button', { name: 'Create API key' }))
  expect(await screen.findByLabelText('API key')).toHaveValue('one-time-secret')
  expect(screen.getByLabelText('API key')).toHaveFocus()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/account/keys',
    expect.objectContaining({
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-TokenInsights-Dataset-ID': mockBootstrap.datasetId,
      },
      body: JSON.stringify({ label: 'Work laptop', permissions: ['read', 'ingest'] }),
    }),
  )
  await userEvent.click(screen.getByRole('button', { name: 'Done' }))
  expect(screen.queryByLabelText('API key')).not.toBeInTheDocument()
  expect(screen.getByLabelText('Key name')).toHaveFocus()
  await userEvent.click(screen.getByRole('button', { name: 'Revoke Work laptop' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Revoke Work laptop' })).toBeDisabled(),
  )
  expect(screen.getByRole('heading', { name: 'Account' })).toBeVisible()
  for (const [, init] of fetcher.mock.calls.filter(
    ([input]) => typeof input === 'string' && input.includes('/account'),
  )) {
    expect(init?.headers).toEqual(
      expect.objectContaining({ 'X-TokenInsights-Dataset-ID': mockBootstrap.datasetId }),
    )
  }
  client.clear()
})

it('password change removes cached account data and returns to login', async () => {
  const { client, fetcher } = fixture()
  fireEvent.change(await screen.findByLabelText('Current password'), {
    target: { value: ' current password ' },
  })
  fireEvent.change(screen.getByLabelText('New password'), {
    target: { value: ' new password phrase ' },
  })
  fireEvent.change(screen.getByLabelText('Confirm new password'), {
    target: { value: ' new password phrase ' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent('Password changed.')
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/account/password',
    expect.objectContaining({
      headers: {
        'Content-Type': 'application/json',
        'X-TokenInsights-Dataset-ID': mockBootstrap.datasetId,
      },
      body: JSON.stringify({
        currentPassword: ' current password ',
        password: ' new password phrase ',
      }),
    }),
  )
  expect(client.getQueryData(['account', mockBootstrap.datasetId])).toBeUndefined()
  expect(client.getQueryData(['account-keys', mockBootstrap.datasetId])).toBeUndefined()
  client.clear()
})

it('signup appears only when enabled and preserves password whitespace', async () => {
  const { client, fetcher } = fixture({ signedIn: false, signupEnabled: true, path: '/tokens' })
  await userEvent.click(await screen.findByRole('button', { name: 'Create an account' }))
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'Alice' } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: ' secret passphrase ' } })
  await userEvent.click(screen.getByRole('button', { name: 'Create account' }))
  expect(await screen.findByLabelText('Total tokens: 647,000')).toBeVisible()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/auth/signup',
    expect.objectContaining({
      body: JSON.stringify({ username: 'Alice', password: ' secret passphrase ' }),
    }),
  )
  client.clear()
})

it('wrong current password preserves the session and allows retry', async () => {
  const { client } = fixture({ rejectMutation: { path: '/account/password', status: 400 } })
  fireEvent.change(await screen.findByLabelText('Current password'), {
    target: { value: 'wrong current password' },
  })
  fireEvent.change(screen.getByLabelText('New password'), {
    target: { value: 'new password phrase' },
  })
  fireEvent.change(screen.getByLabelText('Confirm new password'), {
    target: { value: 'new password phrase' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Change password' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Check your current password and try again.',
  )
  expect(screen.getByRole('heading', { name: 'Account' })).toBeVisible()
  expect(client.getQueryData(['account', mockBootstrap.datasetId])).toBeDefined()
  client.clear()
})

for (const status of [401, 409])
  it(`session rejection ${status} during key revocation hides the one-time secret and clears account data`, async () => {
    const { client } = fixture({ rejectMutation: { path: '/account/keys/key-1', status } })
    fireEvent.change(await screen.findByLabelText('Key name'), { target: { value: 'Work laptop' } })
    await userEvent.click(screen.getByRole('button', { name: 'Create API key' }))
    expect(await screen.findByLabelText('API key')).toHaveValue('one-time-secret')
    await userEvent.click(screen.getByRole('button', { name: 'Revoke Work laptop' }))
    expect(await screen.findByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
    expect(screen.queryByLabelText('API key')).not.toBeInTheDocument()
    expect(client.getQueryData(['account', mockBootstrap.datasetId])).toBeUndefined()
    expect(client.getQueryData(['account-keys', mockBootstrap.datasetId])).toBeUndefined()
    client.clear()
  })

it('a shared-cookie dataset change during settings reads clears cached account data and requests sign in', async () => {
  const { client, fetcher } = fixture({ rejectRead: '/account/keys' })
  expect(await screen.findByRole('heading', { name: 'Sign in to TokenInsights' })).toBeVisible()
  expect(client.getQueryData(['account', mockBootstrap.datasetId])).toBeUndefined()
  expect(client.getQueryData(['account-keys', mockBootstrap.datasetId])).toBeUndefined()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/v2/account/keys',
    expect.objectContaining({ headers: { 'X-TokenInsights-Dataset-ID': mockBootstrap.datasetId } }),
  )
  client.clear()
})

it('password bounds count Unicode characters without trimming', () => {
  expect(validPassword('🔑'.repeat(15))).toBe(true)
  expect(validPassword('🔑'.repeat(128))).toBe(true)
  expect(validPassword('🔑'.repeat(129))).toBe(false)
  expect(validPassword(' '.repeat(15))).toBe(true)
  expect(validPassword('short')).toBe(false)
})
