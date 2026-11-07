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

Keep it simple and honest: headline, install command, dashboard mock, three facts. Claims
come from the docs. No star counts, logos, benchmarks or testimonials. Every figure in the
dashboard mock (`src/site/DashboardMock.tsx`) is labelled as demo data, and the mock is kept
in step with the app by hand because this site has no dependency on it.
