import { backendArguments, connectBackend } from './backend.ts'
import type { BackendConnection } from './backend.ts'

// Every request owns its exact target and capture policy. Failure stays visible.
export function connectTarget(
  executable: string,
  onExit: () => void,
  link?: string,
  connect: typeof connectBackend = connectBackend,
): Promise<BackendConnection> {
  return connect(executable, onExit, backendArguments(link))
}
