// deno-fmt-ignore-file
// biome-ignore format: generated types do not need formatting
// prettier-ignore
import type { PathsForPages, GetConfigResponse } from 'waku/router'

// prettier-ignore
import type { getConfig as File_Index_getConfig } from './pages/index'

// prettier-ignore
type Page =
  | { path: '/docs/commands'; render: 'static' }
  | { path: '/docs/getting-started'; render: 'static' }
  | { path: '/docs'; render: 'static' }
  | { path: '/docs/install'; render: 'static' }
  | { path: '/docs/local-usage'; render: 'static' }
  | { path: '/docs/maintenance'; render: 'static' }
  | { path: '/docs/privacy'; render: 'static' }
  | { path: '/docs/remote'; render: 'static' }
  | ({ path: '/' } & GetConfigResponse<typeof File_Index_getConfig>)

// prettier-ignore
declare module 'waku/router' {
  interface RouteConfig {
    paths: PathsForPages<Page>
  }
  interface CreatePagesConfig {
    pages: Page
  }
}
