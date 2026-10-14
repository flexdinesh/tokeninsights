# Public documentation website

Independent [Hugo](https://gohugo.io/) site using [Hextra](https://imfing.github.io/hextra/). Hugo Extended is pinned in the root `mise.toml`; Hextra is pinned in this directory's `go.mod` and `go.sum`. No pnpm workspace or Node runtime is required to build or serve this site.

From the repository root:

```sh
mise run dev:docs
```

Open <http://localhost:1313/>. Hugo watches content and configuration changes and live reloads the browser. Stop with Ctrl+C. Mise installs the required tools when missing; the first Hugo run downloads the pinned theme.

Build static HTML, CSS, JavaScript, and assets:

```sh
mise run build:docs
```

Output lives in `docs-website/public/` and is ignored by Git. Draft pages appear in the local preview but are excluded from the static build.

Edit public user guides in `content/`. Set `title` and `weight` in each page's YAML front matter to control sidebar labels and order. Follow the existing Markdown pages; custom layouts and assets can be added later.

Internal architecture, ADRs, and other engineering docs stay in the repository's `docs/` directory. This site has no dependency on the application, its databases, or its browser build. Deployment is not configured.
