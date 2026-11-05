// Exercise real host product builds and prebuilt installation, never source installs.
import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { buildProducts } from './build-products.ts'
import { checkArtifacts } from './check-artifacts.ts'
import { workspacePath } from './workspace.ts'

const fixture = await mkdtemp(join(tmpdir(), 'tokeninsights-distribution-'))
const out = join(fixture, 'release')
const metadata = { version: '0.0.0', commit: '1'.repeat(40), date: '2026-11-03T00:00:00Z' }
const env: NodeJS.ProcessEnv = { ...process.env }
for (const name of Object.keys(env)) if (name.startsWith('TOKENINSIGHTS_')) delete env[name]
Object.assign(env, {
  HOME: fixture,
  XDG_CONFIG_HOME: join(fixture, 'config'),
  XDG_DATA_HOME: join(fixture, 'data'),
  XDG_CACHE_HOME: join(fixture, 'cache'),
  XDG_STATE_HOME: join(fixture, 'state'),
  XDG_RUNTIME_DIR: join(fixture, 'runtime'),
  CODEX_HOME: join(fixture, 'codex'),
  CLAUDE_CONFIG_DIR: join(fixture, 'claude'),
  TOKENINSIGHTS_DOWNLOAD_URL: `file://${out}`,
})
try {
  checkArtifacts()
  await buildProducts(out, metadata)
  const { writeChecksums } = await import('./build-products.ts')
  await writeChecksums(out)
  for (const script of ['install.sh', 'install-desktop.sh'])
    execFileSync('sh', [workspacePath(script)], { env, stdio: 'inherit' })
  const installed = join(fixture, 'data/tokeninsights/products')
  for (const [product, binary] of [
    ['cli', 'tokeninsights'],
    [
      'desktop',
      process.platform === 'darwin'
        ? 'TokenInsights Desktop.app/Contents/MacOS/tokeninsights'
        : 'tokeninsights',
    ],
  ]) {
    const output = execFileSync(join(installed, product ?? '', binary ?? ''), ['--version'], {
      env: { ...env, PATH: '' },
      encoding: 'utf8',
    })
    assert.ok(output.includes(metadata.version) && output.includes(metadata.commit), output)
  }
  // Direct package build stays supported when generated embed inputs exist.
  execFileSync('go', ['build', '-o', join(fixture, 'direct-cli'), './cmd/tokeninsights'], {
    cwd: workspacePath('packages/cli'),
    stdio: 'inherit',
  })
  execFileSync(join(fixture, 'direct-cli'), ['--help'], {
    env: { ...env, PATH: '' },
    stdio: 'inherit',
  })
  if (process.platform === 'linux') {
    execFileSync(
      'xvfb-run',
      [
        '-a',
        process.execPath,
        workspacePath('tools/build/src/smoke-desktop.ts'),
        join(installed, 'desktop/tokeninsights-desktop'),
      ],
      { env, stdio: 'inherit' },
    )
  }
  // Serve the installed web dashboard through its bundled backend, then read embedded assets.
  const child = spawn(
    join(installed, 'cli/tokeninsights'),
    ['web', '--open=false', '--port', '0'],
    {
      env: { ...env, PATH: '' },
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  )
  let output = ''
  const stopped = new Promise<void>((resolve, reject) => {
    child.once('error', reject)
    child.once('exit', (code) =>
      code === 0 ? resolve() : reject(new Error(`web exited ${code}: ${output}`)),
    )
  })
  child.stderr.on('data', (data: Buffer) => {
    output += data.toString()
  })
  try {
    const dashboard = await new Promise<string>((resolve, reject) => {
      let text = ''
      const timeout = setTimeout(
        () => reject(new Error(`dashboard readiness timeout: ${output}`)),
        30000,
      )
      child.stdout.on('data', (data: Buffer) => {
        text += data.toString()
        const url = /^Dashboard: (\S+)$/m.exec(text)?.[1]
        if (url === undefined) return
        clearTimeout(timeout)
        resolve(url)
      })
      child.once('error', (error) => {
        clearTimeout(timeout)
        reject(error)
      })
      child.once('exit', () => {
        clearTimeout(timeout)
        reject(new Error(`web exited before readiness: ${output}`))
      })
    })
    const response = await fetch(dashboard, { signal: AbortSignal.timeout(5000) })
    assert.equal(response.status, 200)
    assert.equal(
      await response.text(),
      await readFile(workspacePath('packages/core/server/static/index.html'), 'utf8'),
    )
  } finally {
    child.kill('SIGINT')
    const timeout = setTimeout(() => child.kill('SIGKILL'), 10000)
    try {
      await stopped
    } finally {
      clearTimeout(timeout)
    }
  }
  console.log('Verified prebuilt products, installers, native window and embedded dashboard.')
} finally {
  await rm(fixture, { recursive: true, force: true })
}
