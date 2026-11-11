// Runs inside the invocation-owned Xvfb display with private user data.
import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { setTimeout } from 'node:timers/promises'
const startupAttempts = 60
const startupInterval = 500
// Desktop's window starts with the app name; the page's own title proves it loaded.
// Native desktop titles its window directly, so callers pass that exact title.
const [executable, title = 'TokenInsights'] = process.argv.slice(2)
if (executable === undefined) throw new Error('usage: smoke-desktop.ts <executable> [window-title]')
const dashboardTitle = `"${title}":`
const child = spawn(executable, [], { stdio: ['ignore', 'pipe', 'pipe'] })
let output = ''
child.stdout.on('data', (bytes: Buffer) => {
  output += bytes.toString()
})
child.stderr.on('data', (bytes: Buffer) => {
  output += bytes.toString()
})
const stopped = new Promise<void>((resolve, reject) => {
  child.once('error', reject)
  child.once('exit', () => resolve())
})
async function dashboardWindows(attempts: number): Promise<string> {
  assert.equal(child.exitCode, null, output)
  const windows = execFileSync('xwininfo', ['-root', '-tree'], { encoding: 'utf8' })
  if (windows.includes(dashboardTitle) || attempts <= 1) return windows
  await setTimeout(startupInterval)
  return dashboardWindows(attempts - 1)
}
try {
  const windows = await dashboardWindows(startupAttempts)
  assert.ok(windows.includes(dashboardTitle), `window ${title} missing: ${windows}\n${output}`)
} finally {
  child.kill('SIGTERM')
  const timeout = globalThis.setTimeout(() => child.kill('SIGKILL'), 5000)
  try {
    await stopped
  } finally {
    clearTimeout(timeout)
  }
}
