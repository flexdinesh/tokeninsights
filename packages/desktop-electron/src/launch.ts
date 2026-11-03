import { bootstrapSchema } from '@tokeninsights/contracts'
import { localTarget } from './links.ts'
import { backendArguments, connectBackend } from './backend.ts'
import type { BackendConnection } from './backend.ts'

const probeTimeoutMs = 5_000

// URL handoffs never fall back to another dataset. A tunnel or incompatible
// local owner can still serve the requested dashboard in the system browser.
export async function connectTarget(
  executable: string,
  onExit: () => void,
  targetURL: string | undefined,
  openBrowser: (url: string) => void,
  connect: typeof connectBackend = connectBackend,
): Promise<BackendConnection | undefined> {
  try {
    return await connect(executable, onExit, backendArguments(targetURL))
  } catch (error) {
    if (targetURL === undefined) throw error
    openBrowser(targetURL)
    return undefined
  }
}

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
