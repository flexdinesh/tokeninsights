# Design System: TokenInsights website

The public site (`/`) and docs (`/docs/*`). The app's contract is the root
[`DESIGN.md`](../DESIGN.md); the two are related, not identical, and share the Split T,
the logo blue, square corners and mild rules.

- **Type:** self-hosted JetBrains Mono for the site and docs. The landing page's dashboard
  mock uses Geist, because that is what the app uses.
- **Color:** full white in light, `#121316` in dark, ink `#22252b` / `#ecedf0`. One accent,
  the logo blue, `#285bd4` / `#82aaff`, for the primary action, links and selection.
- **Rules:** 1px, `#e6e6e8` / `#2a2c31`. Corners are square everywhere except status dots.
- **Tokens:** `src/pages/_root.css`. Vocs loads this file on every route, including the 404
  page, and it maps the site tokens onto Vocs' own variables.

## Landing page

Keep it simple and honest: headline, install command, the desktop app demo, the remote
connection section and three facts. Claims come from the docs. No star counts, logos,
benchmarks or testimonials.

- **Demos:** `src/site/demos/`. The hero (`HeroDemo.tsx`) plays once in view: Refresh runs
  capture and the day's new usage rolls into the readouts, chart and table. The remote demo
  (`RemoteDemo.tsx`) loops two connections from the laptop's terminal: a typed
  `tokeninsights connect ssh://you@devbox`, then the pasted connect command that
  `tokeninsights remote` printed on devbox (random demo pin and token). Each opens a desktop
  window on devbox's backend; Cmd+W closes it and Cmd+K clears the terminal. Reduced motion shows each
  demo's finished state. Both are labelled demo data and are kept in step with
  `packages/dashboard` by hand (labels, header connection line, nav icons from
  `lucide-react`), because this site has no dependency on the app.
- **Demo kit:** `src/stageshow/` is a vendored subset of the MIT Stageshow kit (license
  inside), patched to drop type assertions and `any`. Its CSS is unlayered so Vocs' layered
  reset cannot override it; demo CSS scopes under `.ti-demo`. Oxlint skips the directory as
  vendored code.
- **Tokens:** the demos map the site tokens onto the kit's (`src/site/demos/app.css`), so
  light and dark follow the page, with square corners and Geist as in the app.
