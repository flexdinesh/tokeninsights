// Exercise real host product builds and prebuilt installation, never source installs.
import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import { chmod, chown, cp, mkdir, mkdtemp, readFile, rm } from 'node:fs/promises'
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
  TOKENINSIGHTS_PORT: '0',
  TOKENINSIGHTS_DOWNLOAD_URL: `file://${out}`,
})
try {
  checkArtifacts()
  await buildProducts(out, metadata)
  const { writeChecksums } = await import('./build-products.ts')
  await writeChecksums(out)
  for (const script of [
    'install.sh',
    'install-server.sh',
    'install-mygo.sh',
    'install-electron.sh',
  ])
    execFileSync('sh', [workspacePath(script)], { env, stdio: 'inherit' })
  const installed = join(fixture, 'data/tokeninsights/products')
  for (const [product, binary] of [
    ['cli', 'tokeninsights'],
    ['server', 'tokeninsights-server'],
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
  // The packaged Electron binary supplies its own runtime without host Node.
  const electron =
    process.platform === 'darwin'
      ? join(installed, 'electron/TokenInsights Electron.app/Contents/MacOS/tokeninsights-electron')
      : join(installed, 'electron/tokeninsights-electron')
  assert.match(
    execFileSync(electron, ['--version'], {
      env: { ...env, PATH: '', ELECTRON_RUN_AS_NODE: '1' },
      encoding: 'utf8',
    }),
    /^v\d+/,
  )
  if (process.platform === 'linux') {
    execFileSync(
      'xvfb-run',
      [
        '-a',
        process.execPath,
        workspacePath('tools/build/src/smoke-native.ts'),
        join(installed, 'mygo/tokeninsights-mygo'),
      ],
      { env, stdio: 'inherit' },
    )
    // Exercise the installed Electron app and rendered dashboard, not merely
    // its runtime version. Keep the production renderer sandbox enabled.
    const gui = await mkdtemp(join(tmpdir(), 'tokeninsights-electron-gui-'))
    const screenshot = join(gui, 'electron.png')
    const ordinaryUser = 65534 // Debian's nobody account inside validation.
    const runAsOrdinaryUser = process.getuid?.() === 0
    try {
      await chmod(fixture, 0o755)
      if (runAsOrdinaryUser) {
        await chown(gui, ordinaryUser, ordinaryUser)
        // Committed snapshots use private directories. The unprivileged smoke
        // runner needs traversal to its public source and installed dependencies.
        for (const path of ['', 'tools', 'tools/build', 'tools/build/src'])
          await chmod(workspacePath(path), 0o755)
      }
      const args = [
        process.execPath,
        workspacePath('tools/build/src/smoke-electron.ts'),
        electron,
        screenshot,
      ]
      execFileSync(
        runAsOrdinaryUser ? '/usr/sbin/runuser' : 'xvfb-run',
        runAsOrdinaryUser
          ? ['--preserve-environment', '-u', 'nobody', '--', 'xvfb-run', '-a', ...args]
          : ['-a', ...args],
        {
          env: {
            ...env,
            HOME: gui,
            XDG_CONFIG_HOME: join(gui, 'config'),
            XDG_DATA_HOME: join(gui, 'data'),
            XDG_CACHE_HOME: join(gui, 'cache'),
            TMPDIR: gui,
          },
          stdio: 'inherit',
        },
      )
      const artifacts = workspacePath('.check-logs')
      await mkdir(artifacts, { recursive: true })
      await cp(screenshot, join(artifacts, 'electron.png'))
    } finally {
      await rm(gui, { recursive: true, force: true })
    }
  }
  // Launch the installed backend through its private connector, then read embedded assets.
  const child = spawn(join(installed, 'cli/tokeninsights-backend'), ['connect', '--json'], {
    env: { ...env, PATH: '' },
    stdio: ['pipe', 'pipe', 'pipe'],
  })
  let output = ''
  const stopped = new Promise<void>((resolve, reject) => {
    child.once('error', reject)
    child.once('exit', (code) =>
      code === 0 ? resolve() : reject(new Error(`connector exited ${code}: ${output}`)),
    )
  })
  child.stderr.on('data', (data: Buffer) => {
    output += data.toString()
  })
  try {
    const descriptor = await new Promise<unknown>((resolve, reject) => {
      let text = ''
      const timeout = setTimeout(
        () => reject(new Error(`backend readiness timeout: ${output}`)),
        30000,
      )
      child.stdout.on('data', (data: Buffer) => {
        text += data.toString()
        const line = text.split('\n')[0]
        if (!text.includes('\n') || line === undefined) return
        clearTimeout(timeout)
        try {
          resolve(JSON.parse(line))
        } catch (error) {
          reject(error)
        }
      })
      child.once('error', (error) => {
        clearTimeout(timeout)
        reject(error)
      })
      child.once('exit', () => {
        clearTimeout(timeout)
        reject(new Error(`backend exited before readiness: ${output}`))
      })
    })
    assert.ok(
      typeof descriptor === 'object' &&
        descriptor !== null &&
        'url' in descriptor &&
        typeof descriptor.url === 'string',
    )
    const response = await fetch(descriptor.url, { signal: AbortSignal.timeout(5000) })
    assert.equal(response.status, 200)
    assert.equal(
      await response.text(),
      await readFile(workspacePath('packages/core/server/static/index.html'), 'utf8'),
    )
  } finally {
    child.stdin.end()
    const timeout = setTimeout(() => child.kill('SIGKILL'), 10000)
    try {
      await stopped
    } finally {
      clearTimeout(timeout)
    }
  }
  console.log(
    'Verified prebuilt products, installers, bundled runtimes, native window and embedded dashboard.',
  )
} finally {
  await rm(fixture, { recursive: true, force: true })
}
