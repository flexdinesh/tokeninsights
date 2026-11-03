import { useQuery } from '@tanstack/react-query'
import {
  browserSessionPasswordMaxCharacters,
  browserSessionPasswordMinCharacters,
  newPasswordMaxCharacters,
  newPasswordMinCharacters,
} from '@tokeninsights/contracts'
import { useDashboardClient } from './client'

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

export function useAuthInfo(enabled: boolean) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['auth-info'],
    queryFn: ({ signal }) => api.getAuthInfo(signal),
    enabled,
    retry: false,
    staleTime: Infinity,
  })
}

export function useAccount(datasetId: string, enabled: boolean) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['account', datasetId],
    queryFn: ({ signal }) => api.getAccount(datasetId, signal),
    enabled,
  })
}

export function useAPIKeys(datasetId: string) {
  const api = useDashboardClient()
  return useQuery({
    queryKey: ['account-keys', datasetId],
    queryFn: ({ signal }) => api.getAPIKeys(datasetId, signal),
  })
}
