import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync, spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  linkSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test, { type TestContext } from 'node:test'
import { workspacePath } from '../src/workspace.ts'
import { archiveRoot, type Product } from '../src/build-products.ts'

function executable(path: string, text: string): void {
  writeFileSync(path, `#!/bin/sh\n${text}\n`, { mode: 0o755 })
  chmodSync(path, 0o755)
}
function fixture(t: TestContext) {
  const root = mkdtempSync(join(tmpdir(), 'tokeninsights-installer-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const home = join(root, 'home')
  const release = join(root, 'release')
  const bin = join(root, 'bin')
  for (const directory of [home, release, bin]) mkdirSync(directory)
  const log = join(root, 'downloads')
  executable(
    join(bin, 'uname'),
    'case "$1" in -s) echo "${TEST_OS:-Linux}" ;; -m) echo "${TEST_ARCH:-x86_64}" ;; esac',
  )
  executable(join(bin, 'sysctl'), 'echo 0')
  executable(
    join(bin, 'curl'),
    `url=; out=; while [ "$#" -gt 0 ]; do case "$1" in -o) shift; out="$1" ;; https:*) url="$1" ;; esac; shift; done\nprintf '%s\\n' "$url" >> "$TEST_DOWNLOAD_LOG"\ncp "$TEST_RELEASE/\${url##*/}" "$out"`,
  )
  for (const runtime of ['go', 'node', 'npm', 'pnpm'])
    executable(join(bin, runtime), 'echo forbidden-runtime >&2; exit 71')
  const env = {
    PATH: `${bin}:/usr/bin:/bin`,
    HOME: home,
    TMPDIR: root,
    TEST_RELEASE: release,
    TEST_DOWNLOAD_LOG: log,
  }
  const run = (script = 'install.sh', args: string[] = [], overrides: NodeJS.ProcessEnv = {}) =>
    spawnSync('sh', [workspacePath(script), ...args], {
      encoding: 'utf8',
      env: { ...env, ...overrides },
    })
  const runPiped = (script = 'install.sh') =>
    spawnSync('sh', ['-s'], {
      input: readFileSync(workspacePath(script), 'utf8'),
      cwd: home,
      encoding: 'utf8',
      env,
    })
  const installed = (product: Product) => join(home, '.local/share/tokeninsights/products', product)
  const archive = (
    product: Product = 'cli',
    version = 'v1',
    os: 'linux' | 'darwin' = 'linux',
    extra?: (path: string) => void,
  ) => {
    const name = archiveRoot(product, { os, arch: 'amd64' })
    const dir = join(root, 'stage', name)
    rmSync(dir, { recursive: true, force: true })
    mkdirSync(dir, { recursive: true })
    const binary = product === 'cli' ? 'tokeninsights' : `tokeninsights-${product}`
    const app = product === 'mygo' ? 'TokenInsights MyGo.app' : 'TokenInsights Electron.app'
    const binaryDir =
      os === 'darwin' && (product === 'mygo' || product === 'electron')
        ? join(dir, app, 'Contents/MacOS')
        : dir
    mkdirSync(binaryDir, { recursive: true })
    executable(join(binaryDir, binary), `echo ${version}`)
    if (product !== 'server') {
      const backendDir =
        product === 'electron'
          ? os === 'darwin'
            ? join(dir, app, 'Contents/Resources')
            : join(dir, 'resources')
          : binaryDir
      mkdirSync(backendDir, { recursive: true })
      executable(join(backendDir, 'tokeninsights-backend'), `echo backend-${version}`)
    }
    if (os === 'darwin' && (product === 'mygo' || product === 'electron'))
      writeFileSync(
        join(dir, app, 'Contents/Info.plist'),
        `<string>com.flexdinesh.tokeninsights.${product}</string>`,
      )
    writeFileSync(join(dir, 'LICENSE'), 'MIT')
    writeFileSync(join(dir, 'build-info.json'), '{}')
    writeFileSync(join(dir, 'product.txt'), `tokeninsights-product-v1 ${product} ${os} amd64\n`)
    extra?.(dir)
    const filename = `${name}.tar.gz`
    execFileSync('tar', ['-czf', join(release, filename), '-C', join(root, 'stage'), name])
    const checksum = `${createHash('sha256')
      .update(readFileSync(join(release, filename)))
      .digest('hex')}  ${filename}\n`
    writeFileSync(join(release, 'checksums.txt'), checksum)
    return { filename, checksum, dir }
  }
  return { root, home, release, bin, log, run, runPiped, installed, archive }
}

void test('piped entrypoint ignores a hostile working-directory helper', (t) => {
  const f = fixture(t)
  f.archive()
  mkdirSync(join(f.home, 'install'))
  executable(join(f.home, 'install/lib.sh'), 'echo hostile-helper >&2; exit 71')
  writeFileSync(join(f.release, 'lib.sh'), readFileSync(workspacePath('install/lib.sh')))
  const result = f.runPiped()
  assert.equal(result.status, 0, result.stderr)
  assert.doesNotMatch(result.stderr, /hostile-helper/)
  assert.match(
    readFileSync(f.log, 'utf8'),
    /raw\.githubusercontent\.com\/flexdinesh\/tokeninsights\/main\/install\/lib\.sh/,
  )
  assert.ok(existsSync(join(f.installed('cli'), 'tokeninsights-backend')))
})

// macOS tar is BSD tar. Linux validation installs both implementations.
const archiveTools =
  process.platform === 'darwin' ? ['/usr/bin/tar'] : ['/usr/bin/tar', '/usr/bin/bsdtar']
for (const tar of archiveTools) {
  void test(`${tar} installs a macOS framework with contained symlink chains and hardlinks`, (t) => {
    const f = fixture(t)
    const framework = 'TokenInsights Electron.app/Contents/Frameworks/Electron Framework.framework'
    f.archive('electron', 'v1', 'darwin', (root) => {
      const version = join(root, framework, 'Versions/A')
      mkdirSync(join(version, 'Resources'), { recursive: true })
      writeFileSync(join(version, 'Electron Framework'), 'framework-binary')
      writeFileSync(join(version, 'Resources/Info.plist'), 'framework-metadata')
      symlinkSync('A', join(root, framework, 'Versions/Current'))
      symlinkSync(
        'Versions/Current/Electron Framework',
        join(root, framework, 'Electron Framework'),
      )
      symlinkSync('Versions/Current/Resources', join(root, framework, 'Resources'))
      linkSync(join(version, 'Electron Framework'), join(version, 'Framework hardlink'))
    })
    executable(join(f.bin, 'tar'), `exec ${tar} "$@"`)
    const result = f.run('install-electron.sh', [], { TEST_OS: 'Darwin' })
    assert.equal(result.status, 0, result.stderr)
    const installed = join(f.installed('electron'), framework)
    assert.equal(readlinkSync(join(installed, 'Versions/Current')), 'A')
    assert.equal(readFileSync(join(installed, 'Electron Framework'), 'utf8'), 'framework-binary')
    assert.equal(
      readFileSync(join(installed, 'Resources/Info.plist'), 'utf8'),
      'framework-metadata',
    )
    assert.equal(
      readFileSync(join(installed, 'Versions/A/Framework hardlink'), 'utf8'),
      'framework-binary',
    )
  })
}

for (const kind of ['relative escape', 'chained escape', 'cycle', 'dangling', 'symlink ancestor']) {
  void test(`rejects ${kind} before replacing an installed product`, (t) => {
    const f = fixture(t)
    f.archive()
    assert.equal(f.run().status, 0)
    f.archive('cli', 'v2', 'linux', (root) => {
      if (kind === 'relative escape') symlinkSync('../../outside', join(root, 'unsafe'))
      if (kind === 'chained escape') {
        mkdirSync(join(root, 'sub'))
        symlinkSync('..', join(root, 'sub/back'))
        symlinkSync('sub/back/../outside', join(root, 'unsafe'))
      }
      if (kind === 'cycle') {
        symlinkSync('two', join(root, 'one'))
        symlinkSync('one', join(root, 'two'))
      }
      if (kind === 'dangling') symlinkSync('missing', join(root, 'unsafe'))
      if (kind === 'symlink ancestor') {
        mkdirSync(join(root, 'actual'))
        writeFileSync(join(root, 'actual/file'), 'data')
        symlinkSync('actual', join(root, 'alias'))
      }
    })
    if (kind === 'symlink ancestor') {
      const archive = join(f.release, 'tokeninsights-cli_linux_amd64.tar.gz')
      const staged = join(f.root, 'stage')
      // Add a file addressed through the symlink. It is contained, but extraction
      // must reject writes through links independently of archive ordering.
      execFileSync('/usr/bin/tar', [
        '-czf',
        archive,
        '-C',
        staged,
        'tokeninsights-cli_linux_amd64',
        'tokeninsights-cli_linux_amd64/alias/file',
      ])
      writeFileSync(
        join(f.release, 'checksums.txt'),
        `${createHash('sha256').update(readFileSync(archive)).digest('hex')}  tokeninsights-cli_linux_amd64.tar.gz\n`,
      )
    }
    const result = f.run()
    assert.notEqual(result.status, 0)
    assert.match(readFileSync(join(f.installed('cli'), 'tokeninsights'), 'utf8'), /echo v1/)
  })
}
void test('default CLI stable and main installers replace a whole product including backend', (t) => {
  const f = fixture(t)
  f.archive()
  const first = f.run()
  assert.equal(first.status, 0, first.stderr)
  assert.equal(
    execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
    'v1',
  )
  assert.match(
    readFileSync(join(f.installed('cli'), 'tokeninsights-backend'), 'utf8'),
    /backend-v1/,
  )
  f.archive('cli', 'v2')
  const next = f.run('install-main.sh')
  assert.equal(next.status, 0, next.stderr)
  assert.equal(readFileSync(join(f.installed('cli'), '.channel'), 'utf8'), 'main\n')
  assert.match(
    readFileSync(join(f.installed('cli'), 'tokeninsights-backend'), 'utf8'),
    /backend-v2/,
  )
  const urls = readFileSync(f.log, 'utf8')
  assert.match(urls, /releases\/latest\/download\/tokeninsights-cli/)
  assert.match(urls, /releases\/download\/main-build\/tokeninsights-cli/)
})
void test('all product entrypoints support main; desktop products coexist', (t) => {
  const f = fixture(t)
  const selected: readonly Product[] = ['server', 'mygo', 'electron']
  for (const product of selected) {
    f.archive(product)
    const result = f.run(`install-${product}.sh`, ['--channel=main'])
    assert.equal(result.status, 0, result.stderr)
    assert.equal(readFileSync(join(f.installed(product), '.channel'), 'utf8'), 'main\n')
  }
  for (const product of ['mygo', 'electron'])
    assert.ok(
      existsSync(join(f.home, '.local/share/applications', `tokeninsights-${product}.desktop`)),
    )
})
void test('upgrades retain the valid launcher without a remove-and-republish gap', (t) => {
  const f = fixture(t)
  f.archive()
  assert.equal(f.run().status, 0)
  f.archive('cli', 'v2')
  executable(
    join(f.bin, 'mv'),
    'case "$1" in *.tokeninsights-link-*) exit 17 ;; *) exec /bin/mv "$@" ;; esac',
  )
  const result = f.run()
  assert.equal(result.status, 0, result.stderr)
  assert.equal(
    execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
    'v2',
  )
})
for (const failure of [
  'checksum',
  'missing checksum',
  'duplicate checksum',
  'download',
  'backend',
  'incompatible',
  'link',
  'layout',
  'copy',
  'replacement',
]) {
  void test(`failed ${failure} preserves the installed product`, (t) => {
    const f = fixture(t)
    f.archive()
    assert.equal(f.run().status, 0)
    const a = f.archive('cli', 'v2', 'linux', (path) => {
      if (failure === 'backend') rmSync(join(path, 'tokeninsights-backend'))
      if (failure === 'incompatible')
        writeFileSync(join(path, 'product.txt'), 'tokeninsights-product-v2 cli linux amd64\n')
      if (failure === 'link') symlinkSync('/etc/passwd', join(path, 'unsafe'))
    })
    if (failure === 'checksum') writeFileSync(join(f.release, a.filename), 'corrupted')
    if (failure === 'missing checksum') writeFileSync(join(f.release, 'checksums.txt'), '')
    if (failure === 'duplicate checksum')
      writeFileSync(join(f.release, 'checksums.txt'), a.checksum.repeat(2))
    if (failure === 'download') rmSync(join(f.release, a.filename))
    if (failure === 'layout') {
      execFileSync('tar', ['-czf', join(f.release, a.filename), '-C', f.release, 'checksums.txt'])
      writeFileSync(
        join(f.release, 'checksums.txt'),
        `${createHash('sha256')
          .update(readFileSync(join(f.release, a.filename)))
          .digest('hex')}  ${a.filename}\n`,
      )
    }
    if (failure === 'copy')
      executable(
        join(f.bin, 'cp'),
        'case "$*" in *extracted*) exit 17 ;; *) exec /bin/cp "$@" ;; esac',
      )
    if (failure === 'replacement')
      executable(
        join(f.bin, 'mv'),
        'case "$1" in *.cli.install-*) exit 17 ;; *) exec /bin/mv "$@" ;; esac',
      )
    const result = f.run()
    assert.notEqual(result.status, 0, result.stdout)
    assert.match(readFileSync(join(f.installed('cli'), 'tokeninsights'), 'utf8'), /echo v1/)
    assert.match(
      readFileSync(join(f.installed('cli'), 'tokeninsights-backend'), 'utf8'),
      /backend-v1/,
    )
    assert.equal(
      execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
      'v1',
    )
  })
}
void test('refuses unowned products and executable paths', (t) => {
  const f = fixture(t)
  f.archive()
  mkdirSync(f.installed('cli'), { recursive: true })
  assert.match(f.run().stderr, /unowned path/)
  rmSync(f.installed('cli'), { recursive: true })
  mkdirSync(join(f.home, '.local/bin'), { recursive: true })
  executable(join(f.home, '.local/bin/tokeninsights'), 'echo foreign')
  assert.match(f.run().stderr, /unowned path/)
})
void test('macOS desktop bundles have distinct identities and support replacement', (t) => {
  const f = fixture(t)
  const selected: readonly Product[] = ['mygo', 'electron']
  for (const product of selected) {
    f.archive(product, 'v1', 'darwin')
    const result = f.run(`install-${product}.sh`, [], { TEST_OS: 'Darwin' })
    assert.equal(result.status, 0, result.stderr)
    f.archive(product, 'v2', 'darwin')
    const upgrade = f.run(`install-${product}.sh`, [], { TEST_OS: 'Darwin' })
    assert.equal(upgrade.status, 0, upgrade.stderr)
  }
  assert.ok(existsSync(join(f.home, 'Applications/TokenInsights MyGo.app')))
  assert.ok(existsSync(join(f.home, 'Applications/TokenInsights Electron.app')))
})
void test('rejects unsupported host and channel before download', (t) => {
  const f = fixture(t)
  assert.match(f.run('install.sh', [], { TEST_OS: 'FreeBSD' }).stderr, /unsupported OS/)
  assert.match(f.run('install.sh', [], { TEST_ARCH: 'i686' }).stderr, /unsupported architecture/)
  assert.match(f.run('install.sh', ['--channel=edge']).stderr, /unknown option/)
  assert.ok(!existsSync(f.log))
})
