import type { Plugin } from '@opencode/plugin'
import { setupCompletion } from './lifecycle.ts'

export default {
  id: 'tokeninsights',
  setup: setupCompletion,
} satisfies Plugin.Plugin
