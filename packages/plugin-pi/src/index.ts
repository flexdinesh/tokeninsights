import type { ExtensionAPI } from '@earendil-works/pi-coding-agent'
import { registerNativeCompletion } from './lifecycle.ts'

export default function tokeninsights(pi: ExtensionAPI): void {
  registerNativeCompletion(pi)
}
