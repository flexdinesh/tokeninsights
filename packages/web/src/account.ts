import { useQuery } from '@tanstack/react-query'
import type { z } from 'zod'
import { AuthenticationRequired, request } from './api'
import {
  accountUserSchema,
  apiKeyCreatedSchema,
  apiKeyListSchema,
  authInfoSchema,
  errorSchema,
} from './contracts'

export const passwordMinCharacters = 15
export const passwordMaxCharacters = 128

export function validPassword(password: string): boolean {
  const length = Array.from(password).length
  return length >= passwordMinCharacters && length <= passwordMaxCharacters
}

export const useAuthInfo = (enabled: boolean) =>
  useQuery({
    queryKey: ['auth-info'],
    queryFn: ({ signal }) => request('/api/v2/auth', authInfoSchema, signal),
    enabled,
    retry: false,
    staleTime: Infinity,
  })

export const useAccount = (datasetId: string, enabled: boolean) =>
  useQuery({
    queryKey: ['account', datasetId],
    queryFn: ({ signal }) => request('/api/v2/account', accountUserSchema, signal, datasetId),
    enabled,
  })

export const useAPIKeys = (datasetId: string) =>
  useQuery({
    queryKey: ['account-keys', datasetId],
    queryFn: ({ signal }) => request('/api/v2/account/keys', apiKeyListSchema, signal, datasetId),
  })

async function mutate(
  path: string,
  method: string,
  body?: unknown,
  datasetId?: string,
): Promise<Response> {
  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: {
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...(datasetId === undefined ? {} : { 'X-TokenInsights-Dataset-ID': datasetId }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (response.status === 401 || (datasetId !== undefined && response.status === 409))
    throw new AuthenticationRequired()
  if (!response.ok) {
    let error: unknown
    try {
      error = await response.json()
    } catch {
      /* Fall back to the response status. */
    }
    const parsed = errorSchema.safeParse(error)
    throw new Error(parsed.success ? parsed.data.message : `Request failed (${response.status})`)
  }
  return response
}

export async function signup(credentials: { username: string; password: string }): Promise<void> {
  await mutate('/api/v2/auth/signup', 'POST', credentials)
}

export async function changePassword(
  datasetId: string,
  currentPassword: string,
  password: string,
): Promise<void> {
  await mutate('/api/v2/account/password', 'PUT', { currentPassword, password }, datasetId)
}

export async function createAPIKey(
  datasetId: string,
  body: {
    label: string
    permissions: ('read' | 'ingest')[]
    expiresAt?: string
  },
): Promise<z.output<typeof apiKeyCreatedSchema>> {
  const response = await mutate('/api/v2/account/keys', 'POST', body, datasetId)
  return apiKeyCreatedSchema.parse(await response.json())
}

export async function revokeAPIKey(datasetId: string, tokenId: string): Promise<void> {
  await mutate(
    `/api/v2/account/keys/${encodeURIComponent(tokenId)}`,
    'DELETE',
    undefined,
    datasetId,
  )
}
