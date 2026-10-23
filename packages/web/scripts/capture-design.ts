// Manual design catalog capture: starts the Vite dev server, opens /_design in
// light and dark themes, and writes full-page screenshots to the repository's
// ignored .scratch/design-catalog directory. Not part of check:push.
import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'
import type { Browser } from '@playwright/test'
import { createServer } from 'vite'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const outputDirectory = fileURLToPath(new URL('../../../.scratch/design-catalog/', import.meta.url))
const viewports = [
  { name: 'desktop', width: 1440, height: 1100 },
  { name: 'mobile', width: 390, height: 844 },
]
const themes = ['light', 'dark']

async function capture(
  browser: Browser,
  origin: string,
  viewport: (typeof viewports)[number],
  theme: string,
) {
  const page = await browser.newPage({
    viewport: { width: viewport.width, height: viewport.height },
    reducedMotion: 'reduce',
  })
  try {
    await page.goto(new URL(`/_design?theme=${theme}`, origin).href)
    await page.getByRole('heading', { name: 'Tokens', exact: true }).waitFor()
    await page.evaluate(() => document.fonts.ready)
    const path = `${outputDirectory}${viewport.name}-${theme}.png`
    await page.screenshot({ path, fullPage: true })
    return path
  } finally {
    await page.close()
  }
}

const server = await createServer({ root: webRoot, server: { port: 0, strictPort: false } })
await server.listen()
const origin = server.resolvedUrls?.local[0]
if (!origin) throw new Error('Vite dev server did not report a local URL')

const browser = await chromium.launch()
try {
  await mkdir(outputDirectory, { recursive: true })
  const paths = await Promise.all(
    viewports.flatMap((viewport) =>
      themes.map((theme) => capture(browser, origin, viewport, theme)),
    ),
  )
  for (const path of paths) console.log(path)
} finally {
  await browser.close()
  await server.close()
}
