import { bootstrapSchema } from '@tokeninsights/contracts'
import { localTarget } from './links.ts'

const probeTimeoutMs = 5_000

// A loopback URL may still be a hosted server. Probe only the instance endpoint;
// authentication, network failure and invalid descriptors stay in the browser.
export async function localLaunch(value: string, fetcher: typeof fetch = fetch): Promise<boolean> {
  if (!localTarget(value)) return false
  try {
    const response = await fetcher(new URL('/api/v2/instance', value), {
      redirect: 'error',
      signal: AbortSignal.timeout(probeTimeoutMs),
    })
    if (!response.ok) return false
    const body: unknown = await response.json()
    const instance = bootstrapSchema.parse(body)
    return instance.serverKind === 'personal'
  } catch {
    return false
  }
}
