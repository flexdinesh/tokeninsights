// Runs inside the invocation-owned Xvfb display with private user data.
import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { setTimeout } from 'node:timers/promises'
const executable = process.argv[2]
if (executable === undefined) throw new Error('native executable required')
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
try {
  // Give GTK a bounded startup interval, then inspect the real X11 window tree.
  await setTimeout(5000)
  assert.equal(child.exitCode, null, output)
  const windows = execFileSync('xwininfo', ['-root', '-tree'], { encoding: 'utf8' })
  assert.match(windows, /TokenInsights MyGo/, `native window missing: ${windows}\n${output}`)
} finally {
  child.kill('SIGTERM')
  const timeout = globalThis.setTimeout(() => child.kill('SIGKILL'), 5000)
  try {
    await stopped
  } finally {
    clearTimeout(timeout)
  }
}
