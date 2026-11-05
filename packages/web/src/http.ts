import type { z } from 'zod'
import { errorSchema } from '@tokeninsights/contracts'

export type SendOptions = {
  method?: 'GET' | 'POST'
  body?: unknown
  signal?: AbortSignal
  // Prefix for the status-based message when the server sends no error body.
  failure?: string
}

async function errorMessage(response: Response, failure: string): Promise<string> {
  let body: unknown
  try {
    body = await response.json()
  } catch {
    /* Fall back to the response status. */
  }
  const parsed = errorSchema.safeParse(body)
  return parsed.success ? parsed.data.message : `${failure} (${response.status})`
}

// Sends a same-origin API request and rejects transport and server failures.
export async function send(path: string, options: SendOptions = {}): Promise<Response> {
  const { method = 'GET', body, signal, failure = 'Request failed' } = options
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const response = await fetch(path, {
    signal,
    method,
    credentials: 'same-origin',
    headers: Object.keys(headers).length === 0 ? undefined : headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) throw new Error(await errorMessage(response, failure))
  return response
}

export async function parseBody<T>(response: Response, schema: z.ZodType<T>): Promise<T> {
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new Error('Invalid response from server')
  }
  return schema.parse(body)
}

export async function request<T>(
  path: string,
  schema: z.ZodType<T>,
  signal?: AbortSignal,
): Promise<T> {
  return parseBody(await send(path, { signal }), schema)
}
