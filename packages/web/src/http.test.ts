import { afterEach, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { changeBrowserSession } from './api'
import { AuthenticationRequired, request } from './http'

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

it('surfaces server error messages when changing the browser session', async () => {
  respond(
    new Response(
      JSON.stringify({ code: 'invalid_request', message: 'Username must not be empty.' }),
      { status: 400 },
    ),
  )
  await expect(changeBrowserSession({ username: '', password: 'secret-password' })).rejects.toThrow(
    'Username must not be empty.',
  )
})

it('falls back to the session status without an error body', async () => {
  respond(new Response(null, { status: 500 }))
  await expect(changeBrowserSession()).rejects.toThrow('Sign in failed (500)')
})

it('treats rejected credentials as an authentication requirement', async () => {
  respond(new Response(null, { status: 401 }))
  await expect(
    changeBrowserSession({ username: 'alice', password: 'wrong-password' }),
  ).rejects.toBeInstanceOf(AuthenticationRequired)
})
