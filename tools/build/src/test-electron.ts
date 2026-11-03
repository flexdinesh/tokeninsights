// Host-only packaged renderer proof. Dagger disallows unprivileged namespaces.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { buildProducts, sourceMetadata, writeChecksums } from './build-products.ts'
import { checkArtifacts } from './check-artifacts.ts'
import { workspacePath, workspaceRoot } from './workspace.ts'

function head(): string {
  return execFileSync('git', ['rev-parse', 'HEAD'], { cwd: workspaceRoot, encoding: 'utf8' }).trim()
}

function requireClean(): void {
  const status = execFileSync('git', ['status', '--porcelain', '--untracked-files=no'], {
    cwd: workspaceRoot,
    encoding: 'utf8',
  })
  assert.equal(status, '', 'Desktop HEAD validation requires clean tracked files.')
}

export async function smokeElectronProduct(): Promise<void> {
  if (process.getuid?.() === 0)
    throw new Error('Run the sandboxed desktop check as an ordinary user.')
  const fixture = await mkdtemp(join(tmpdir(), 'tokeninsights-electron-host-'))
  const out = join(fixture, 'release')
  const artifacts = workspacePath('.check-logs')
  const screenshot = join(artifacts, 'electron.png')
  const metadata = sourceMetadata('0.0.0')
  const env: NodeJS.ProcessEnv = { ...process.env }
  for (const name of Object.keys(env)) if (name.startsWith('TOKENINSIGHTS_')) delete env[name]
  delete env.ELECTRON_RUN_AS_NODE
  Object.assign(env, {
    HOME: fixture,
    XDG_CONFIG_HOME: join(fixture, 'config'),
    XDG_DATA_HOME: join(fixture, 'data'),
    XDG_CACHE_HOME: join(fixture, 'cache'),
    XDG_STATE_HOME: join(fixture, 'state'),
    XDG_RUNTIME_DIR: join(fixture, 'runtime'),
    CODEX_HOME: join(fixture, 'codex'),
    CLAUDE_CONFIG_DIR: join(fixture, 'claude'),
    TMPDIR: fixture,
    TOKENINSIGHTS_PORT: '0',
    TOKENINSIGHTS_DOWNLOAD_URL: `file://${out}`,
    DEBUG: 'pw:browser',
  })
  try {
    await mkdir(join(fixture, 'runtime'), { mode: 0o700 })
    await mkdir(artifacts, { recursive: true })
    await Promise.all(
      ['electron.png', 'electron.json'].map((file) => rm(join(artifacts, file), { force: true })),
    )
    checkArtifacts()
    await buildProducts(out, metadata, ['electron'])
    await writeChecksums(out)
    execFileSync('sh', [workspacePath('install-electron.sh')], { env, stdio: 'inherit' })
    const executable = join(
      fixture,
      'data/tokeninsights/products/electron',
      process.platform === 'darwin'
        ? 'TokenInsights Electron.app/Contents/MacOS/tokeninsights-electron'
        : 'tokeninsights-electron',
    )
    execFileSync(
      process.execPath,
      [workspacePath('tools/build/src/smoke-electron.ts'), executable, screenshot],
      {
        cwd: workspaceRoot,
        env,
        stdio: 'inherit',
      },
    )
    assert.equal(head(), metadata.commit, 'HEAD changed during desktop validation.')
    await writeFile(
      join(artifacts, 'electron.json'),
      `${JSON.stringify({ commit: metadata.commit, platform: process.platform, arch: process.arch, chromiumSandbox: true, screenshot }, null, 2)}\n`,
    )
    console.log(
      `Verified packaged Electron dashboard with Chromium sandbox enabled: ${metadata.commit} ${process.platform}/${process.arch}`,
    )
  } finally {
    await rm(fixture, { recursive: true, force: true })
  }
}

if (import.meta.main) {
  requireClean()
  const commit = head()
  await smokeElectronProduct()
  requireClean()
  assert.equal(head(), commit, 'HEAD changed during desktop validation.')
}
