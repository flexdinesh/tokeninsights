// deno-fmt-ignore-file
// biome-ignore format: generated types do not need formatting
// prettier-ignore
import type { PathsForPages } from 'waku/router'

// prettier-ignore
type Page =
  | { path: '/commands'; render: 'static' }
  | { path: '/getting-started'; render: 'static' }
  | { path: '/'; render: 'static' }
  | { path: '/install'; render: 'static' }
  | { path: '/local-usage'; render: 'static' }
  | { path: '/maintenance'; render: 'static' }
  | { path: '/privacy'; render: 'static' }
  | { path: '/self-hosting'; render: 'static' }
  | { path: '/sync'; render: 'static' }

// prettier-ignore
declare module 'waku/router' {
  interface RouteConfig {
    paths: PathsForPages<Page>
  }
  interface CreatePagesConfig {
    pages: Page
  }
}
