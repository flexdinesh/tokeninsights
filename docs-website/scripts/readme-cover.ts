// Films the landing page's HeroDemo into the README cover: one animated WebP
// per color scheme in assets/readme/. The demo plays slowed down while
// screenshots run, frames are resampled onto a fixed scene-time grid, the
// result holds, then cross-fades back to the first frame so the loop has a
// visible reset.
//
//   node scripts/readme-cover.ts [--url http://localhost:5173/]
//
// Without --url it starts `vocs dev` on a free port and stops it afterwards.
// Needs Chromium: `mise run setup:browser`.

import { spawn, type ChildProcess } from 'node:child_process'
import { mkdir, stat } from 'node:fs/promises'
import { createServer } from 'node:net'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseArgs } from 'node:util'
import { chromium, type Browser } from 'playwright'
import sharp from 'sharp'

const SITE_DIR = join(dirname(fileURLToPath(import.meta.url)), '..')
const OUT_DIR = join(SITE_DIR, '..', 'assets', 'readme')
const STAGE = '.ti-hero-demo'
type Scheme = 'light' | 'dark'
const SCHEMES: Scheme[] = ['light', 'dark']

const VIEWPORT = { width: 1600, height: 1000 }
const DEVICE_SCALE = 2
// Scene time is slowed so each screenshot lands within a few scene ms
const SLOW = 6
// HeroDemo's script runs about 5.4 s; the tail lets the number rolls settle
const SCENE_MS = 6500
const HOLD_MS = 3000
const FADE_MS = 400
const SERVER_TIMEOUT_MS = 60_000
const SIZE_BUDGET_BYTES = 3 * 1024 * 1024
const MS_PER_SECOND = 1000
const RGBA = 4

const { values } = parseArgs({
  options: {
    url: { type: 'string' },
    width: { type: 'string', default: '1600' },
    fps: { type: 'string', default: '20' },
    quality: { type: 'string', default: '80' },
  },
})
const outWidth = Number(values.width)
const fps = Number(values.fps)
const quality = Number(values.quality)
const frameMs = MS_PER_SECOND / fps

type Shot = { sceneMs: number; png: Buffer }
type Frame = { pixels: Buffer; delay: number }

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      const port = typeof address === 'object' && address ? address.port : 0
      server.close(() => resolve(port))
    })
  })
}

async function waitForServer(url: string): Promise<void> {
  const deadline = Date.now() + SERVER_TIMEOUT_MS
  while (Date.now() < deadline) {
    try {
      const response = await fetch(url)
      if (response.ok) return
    } catch {
      // not listening yet
    }
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error(`docs dev server did not answer at ${url}`)
}

async function startServer(): Promise<{ url: string; server: ChildProcess }> {
  const port = await freePort()
  const server = spawn(
    join(SITE_DIR, 'node_modules', '.bin', 'vocs'),
    ['dev', '--port', String(port), '--host', '127.0.0.1'],
    { cwd: SITE_DIR, stdio: 'ignore' },
  )
  const url = `http://127.0.0.1:${port}/`
  await waitForServer(url)
  return { url, server }
}

async function film(browser: Browser, url: string, scheme: Scheme): Promise<Shot[]> {
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: DEVICE_SCALE,
    colorScheme: scheme,
    reducedMotion: 'no-preference',
  })
  const page = await context.newPage()
  // The vendored stage reads window.__stageshow.slow (src/stageshow/stage.tsx)
  await page.addInitScript({ content: `window.__stageshow = { slow: ${SLOW} }` })
  const cdp = await context.newCDPSession(page)
  await cdp.send('Animation.enable')
  await cdp.send('Animation.setPlaybackRate', { playbackRate: 1 / SLOW })
  await page.goto(url, { waitUntil: 'networkidle' })
  const stage = page.locator(STAGE).first()
  await stage.waitFor({ state: 'visible' })

  const shots: Shot[] = []
  const start = Date.now()
  while (Date.now() - start < SCENE_MS * SLOW) {
    const taken = Date.now()
    const png = await stage.screenshot({ animations: 'allow' })
    shots.push({ sceneMs: (taken - start) / SLOW, png })
  }
  await context.close()
  return shots
}

async function toPixels(png: Buffer): Promise<{ data: Buffer; width: number; height: number }> {
  const { data, info } = await sharp(png)
    .resize({ width: outWidth })
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true })
  return { data, width: info.width, height: info.height }
}

function blend(from: Buffer, to: Buffer, amount: number): Buffer {
  const out = Buffer.alloc(from.length)
  for (let i = 0; i < from.length; i++) {
    out[i] = Math.round((from[i] ?? 0) * (1 - amount) + (to[i] ?? 0) * amount)
  }
  return out
}

async function encode(shots: Shot[], file: string): Promise<number> {
  // Resample onto the scene-time grid: each slot shows the latest shot taken by then
  const frames: Frame[] = []
  let width = 0
  let height = 0
  let next = 0
  for (let t = 0; t < SCENE_MS; t += frameMs) {
    while (next + 1 < shots.length && (shots[next + 1]?.sceneMs ?? Infinity) <= t) next++
    const shot = shots[next]
    if (!shot) continue
    const pixels = await toPixels(shot.png)
    width = pixels.width
    height = pixels.height
    const last = frames.at(-1)
    if (last && last.pixels.equals(pixels.data)) last.delay += frameMs
    else frames.push({ pixels: pixels.data, delay: frameMs })
  }
  const first = frames[0]
  const last = frames.at(-1)
  if (!first || !last) throw new Error('no frames captured')
  last.delay += HOLD_MS
  const fadeSteps = Math.round(FADE_MS / frameMs)
  for (let step = 1; step < fadeSteps; step++) {
    frames.push({ pixels: blend(last.pixels, first.pixels, step / fadeSteps), delay: frameMs })
  }

  const pngs = await Promise.all(
    frames.map((frame) =>
      sharp(frame.pixels, { raw: { width, height, channels: RGBA } })
        .png()
        .toBuffer(),
    ),
  )
  await sharp(pngs, { join: { animated: true } })
    .webp({
      quality,
      effort: 6,
      loop: 0,
      delay: frames.map((frame) => Math.round(frame.delay)),
    })
    .toFile(file)
  return frames.length
}

async function main(): Promise<void> {
  const started = values.url ? null : await startServer()
  const url = values.url ?? started?.url ?? ''
  const browser = await chromium.launch()
  try {
    await mkdir(OUT_DIR, { recursive: true })
    for (const scheme of SCHEMES) {
      const shots = await film(browser, url, scheme)
      const file = join(OUT_DIR, `cover-${scheme}.webp`)
      const count = await encode(shots, file)
      const { size } = await stat(file)
      const kb = Math.round(size / 1024)
      console.log(
        `${relative(process.cwd(), file)}: ${count} frames, ${shots.length} shots, ${kb} KB`,
      )
      if (size > SIZE_BUDGET_BYTES) {
        console.warn(`  over the 3 MB budget; try --fps 15 or --width 1400`)
      }
    }
  } finally {
    await browser.close()
    started?.server.kill()
  }
}

await main()
