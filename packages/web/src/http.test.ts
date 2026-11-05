import { afterEach, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { httpDashboardClient } from './http-client'
import { request } from './http'

afterEach(() => {
  vi.unstubAllGlobals()
})

function respond(response: Response): void {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() => Promise.resolve(response)),
  )
}

it('reports non-JSON server failures by status', async () => {
  respond(new Response('<html>Bad Gateway</html>', { status: 502 }))
  await expect(request('/api/v2/status', z.unknown())).rejects.toThrow('Request failed (502)')
})

it('surfaces server error messages before parsing success bodies', async () => {
  respond(
    new Response(JSON.stringify({ code: 'unavailable', message: 'Storage is unavailable.' }), {
      status: 503,
    }),
  )
  await expect(request('/api/v2/status', z.unknown())).rejects.toThrow('Storage is unavailable.')
})

it('rejects unparseable success bodies', async () => {
  respond(new Response('not json', { status: 200 }))
  await expect(request('/api/v2/status', z.unknown())).rejects.toThrow(
    'Invalid response from server',
  )
})

it('Refresh sends one same-origin JSON action without connection credentials', async () => {
  const fetcher = vi.fn<typeof fetch>(() => Promise.resolve(new Response(null, { status: 202 })))
  vi.stubGlobal('fetch', fetcher)
  await httpDashboardClient.refresh()
  expect(fetcher).toHaveBeenCalledExactlyOnceWith(
    '/api/viewer/refresh',
    expect.objectContaining({
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    }),
  )
})

it('rejected viewer access reports a connection failure', async () => {
  respond(new Response(null, { status: 401 }))
  await expect(httpDashboardClient.refresh()).rejects.toThrow('Request failed (401)')
})
