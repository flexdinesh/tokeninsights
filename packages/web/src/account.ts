import { useQuery } from '@tanstack/react-query'
import type { z } from 'zod'
import {
  accountUserSchema,
  apiKeyCreatedSchema,
  apiKeyListSchema,
  authInfoSchema,
  browserSessionPasswordMaxCharacters,
  browserSessionPasswordMinCharacters,
  newPasswordMaxCharacters,
  newPasswordMinCharacters,
} from './contracts'
import type { CreateAPIKeyRequest } from './contracts'
import { parseBody, request, send } from './http'

// Signup and password change both accept new passwords; enforce bounds valid for each.
export const passwordMinCharacters = Math.max(
  browserSessionPasswordMinCharacters,
  newPasswordMinCharacters,
)
export const passwordMaxCharacters = Math.min(
  browserSessionPasswordMaxCharacters,
  newPasswordMaxCharacters,
)

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

export async function signup(credentials: { username: string; password: string }): Promise<void> {
  await send('/api/v2/auth/signup', { method: 'POST', body: credentials })
}

export async function changePassword(
  datasetId: string,
  currentPassword: string,
  password: string,
): Promise<void> {
  await send('/api/v2/account/password', {
    method: 'PUT',
    body: { currentPassword, password },
    datasetId,
  })
}

export async function createAPIKey(
  datasetId: string,
  body: CreateAPIKeyRequest,
): Promise<z.output<typeof apiKeyCreatedSchema>> {
  const response = await send('/api/v2/account/keys', { method: 'POST', body, datasetId })
  return parseBody(response, apiKeyCreatedSchema)
}

export async function revokeAPIKey(datasetId: string, tokenId: string): Promise<void> {
  await send(`/api/v2/account/keys/${encodeURIComponent(tokenId)}`, {
    method: 'DELETE',
    datasetId,
  })
}
