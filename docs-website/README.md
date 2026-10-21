# Public documentation website

[Vocs](https://vocs.dev/) site, a pnpm workspace package (`@tokeninsights/docs`). Vocs, Waku, Vite, and React are pinned in `package.json`; React 19.3 is required by Vocs and is independent of the dashboard's React version.

From the repository root:

```sh
pnpm run dev:docs
```

Open <http://localhost:5173/>. Vocs watches pages and configuration and live reloads the browser. Stop with Ctrl+C.

Build static HTML, CSS, JavaScript, and assets:

```sh
pnpm run build:docs
```

`renderStrategy: 'full-static'` writes plain files to `docs-website/dist/public/`, ignored by Git. The build fails on dead internal links. `mise run dev:docs` and `mise run build:docs` delegate to the same scripts. The pre-push check does not build docs.

Edit public user guides in `src/pages/` as Markdown or MDX. Each page's `# H1` sets its title; `vocs.config.ts` controls the sidebar labels and order. Run `pnpm run format` and `pnpm run lint` from the root to format and lint pages and configuration.

Internal architecture, ADRs, and other engineering docs stay in the repository's `docs/` directory. This site has no dependency on the application, its databases, or its browser build. Deployment is not configured.
