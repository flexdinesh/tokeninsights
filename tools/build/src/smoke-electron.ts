// Run as an ordinary host user. Headless Ozone supplies a private compositor on
// Linux hosts without a display; it still renders the actual BrowserWindow.
import assert from 'node:assert/strict'
import { _electron } from 'playwright'

const executable = process.argv[2]
const screenshot = process.argv[3]
if (executable === undefined || screenshot === undefined)
  throw new Error('packaged Electron executable and screenshot path required')

const env: Record<string, string> = {}
for (const [key, value] of Object.entries(process.env)) if (value !== undefined) env[key] = value
delete env.ELECTRON_RUN_AS_NODE
// The app and its Go backend must work without host JavaScript tools on PATH.
env.PATH = ''
const application = await _electron.launch({
  executablePath: executable,
  env,
  args:
    process.platform === 'linux' && !env.DISPLAY && !env.WAYLAND_DISPLAY
      ? ['--ozone-platform=headless']
      : [],
  chromiumSandbox: true,
  timeout: 30_000,
})
try {
  const page = await application.firstWindow()
  await page.setViewportSize({ width: 1280, height: 850 })
  await page.getByRole('region', { name: 'Dashboard filters' }).waitFor({ state: 'visible' })
  await page
    .getByText('No usage saved yet. Use Refresh to collect usage.', { exact: true })
    .waitFor({ state: 'visible' })
  assert.match(await page.title(), /TokenInsights/)
  assert.deepEqual(
    await application.evaluate(({ app }) => ({
      enableSandboxSwitch: app.commandLine.hasSwitch('enable-sandbox'),
      noSandboxSwitch: app.commandLine.hasSwitch('no-sandbox'),
    })),
    { enableSandboxSwitch: true, noSandboxSwitch: false },
  )
  assert.deepEqual(
    await page.evaluate(() => ({
      hasRequire: 'require' in globalThis,
      hasProcess: 'process' in globalThis,
    })),
    { hasRequire: false, hasProcess: false },
  )
  await page.screenshot({ path: screenshot })
} finally {
  await application.close()
}
