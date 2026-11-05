# Public documentation website

User guides: [install](src/pages/install.mdx), [getting started](src/pages/getting-started.mdx), [local usage](src/pages/local-usage.mdx), [commands](src/pages/commands.mdx), [remote connections](src/pages/remote.mdx), [privacy](src/pages/privacy.mdx), [maintenance](src/pages/maintenance.mdx).

[Vocs](https://vocs.dev/) site, a pnpm workspace package (`@tokeninsights/docs`). Vocs, Waku, Vite, and React are pinned in `package.json`; React 19.3 is required by Vocs and is independent of the dashboard's React version.

From the repository root:

```sh
mise run dev:docs
```

Open <http://localhost:5173/>. Vocs watches pages and configuration and live reloads the browser. Stop with Ctrl+C.

Build static HTML, CSS, JavaScript, and assets:

```sh
mise run build:docs
```

`renderStrategy: 'full-static'` writes plain files to `docs-website/dist/public/`, ignored by Git. The build fails on dead internal links. Both tasks call this package's `dev` and `build` scripts. Before PR creation/update, root runs the docs build plus full `check:pr` and sandboxed host `check:desktop` on clean committed HEAD. Pre-push runs no checks.

Edit public user guides in `src/pages/` as Markdown or MDX. Each page's `# H1` sets its title; `vocs.config.ts` controls the sidebar labels and order. Run `mise run format` and `mise run lint` from the root to format and lint pages and configuration.

Internal architecture, ADRs, and other engineering docs stay in the repository's `docs/` directory. This site has no dependency on the application, its databases, or its browser build. Deployment is not configured.
