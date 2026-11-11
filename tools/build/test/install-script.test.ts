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
import { desktopIdentities } from '../src/build-desktop.ts'

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
  const runCli = (script = 'install.sh', args: string[] = [], overrides: NodeJS.ProcessEnv = {}) =>
    run(script, ['--cli-only', ...args], overrides)
  const runPiped = (script = 'install.sh', args: string[] = []) =>
    spawnSync('sh', ['-s', '--', ...args], {
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
    arch: 'amd64' | 'arm64' = 'amd64',
  ) => {
    const name = archiveRoot(product, { os, arch })
    const dir = join(root, 'stage', name)
    rmSync(dir, { recursive: true, force: true })
    mkdirSync(dir, { recursive: true })
    const identity = product === 'cli' ? undefined : desktopIdentities[product]
    const binary = identity?.binary ?? 'tokeninsights'
    const binaryDir =
      os === 'darwin' && identity !== undefined ? join(dir, identity.app, 'Contents/MacOS') : dir
    mkdirSync(binaryDir, { recursive: true })
    executable(join(binaryDir, binary), `echo ${version}`)
    executable(join(binaryDir, 'tokeninsights-backend'), `echo backend-${version}`)
    if (product === 'desktop') executable(join(binaryDir, 'tokeninsights'), `echo cli-${version}`)
    if (os === 'darwin' && identity !== undefined)
      writeFileSync(
        join(dir, identity.app, 'Contents/Info.plist'),
        `<string>${identity.identifier}</string>`,
      )
    writeFileSync(join(dir, 'LICENSE'), 'MIT')
    writeFileSync(join(dir, 'build-info.json'), '{}')
    writeFileSync(join(dir, 'product.txt'), `tokeninsights-product-v1 ${product} ${os} ${arch}\n`)
    extra?.(dir)
    const filename = `${name}.tar.gz`
    execFileSync('tar', ['-czf', join(release, filename), '-C', join(root, 'stage'), name])
    const checksum = `${createHash('sha256')
      .update(readFileSync(join(release, filename)))
      .digest('hex')}  ${filename}\n`
    writeFileSync(join(release, 'checksums.txt'), checksum)
    return { filename, checksum, dir }
  }
  const retiredMyGo = (os: 'linux' | 'darwin') => {
    const product = join(home, '.local/share/tokeninsights/products/mygo')
    const binaryDir =
      os === 'darwin' ? join(product, 'TokenInsights MyGo.app/Contents/MacOS') : product
    mkdirSync(binaryDir, { recursive: true })
    const marker = join(product, '.installed-by-tokeninsights')
    writeFileSync(marker, 'tokeninsights-product:mygo\n')
    const cli = join(binaryDir, 'tokeninsights')
    executable(cli, 'echo retired-cli')
    executable(join(binaryDir, 'tokeninsights-backend'), 'echo retired-backend')
    const launcher =
      os === 'darwin'
        ? join(home, 'Applications/TokenInsights MyGo.app')
        : join(home, '.local/bin/tokeninsights-mygo')
    if (os === 'linux') executable(join(binaryDir, 'tokeninsights-mygo'), 'echo retired-app')
    mkdirSync(join(home, '.local/bin'), { recursive: true })
    mkdirSync(join(home, 'Applications'), { recursive: true })
    symlinkSync(
      os === 'darwin'
        ? join(product, 'TokenInsights MyGo.app')
        : join(binaryDir, 'tokeninsights-mygo'),
      launcher,
    )
    const command = join(home, '.local/bin/tokeninsights')
    symlinkSync(cli, command)
    return { product, marker, cli, command, launcher }
  }
  return { root, home, release, bin, log, run, runCli, runPiped, installed, archive, retiredMyGo }
}

void test('piped entrypoint ignores a hostile working-directory helper', (t) => {
  const f = fixture(t)
  f.archive()
  mkdirSync(join(f.home, 'install'))
  executable(join(f.home, 'install/lib.sh'), 'echo hostile-helper >&2; exit 71')
  writeFileSync(join(f.release, 'lib.sh'), readFileSync(workspacePath('install/lib.sh')))
  const result = f.runPiped('install.sh', ['--cli-only'])
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
    const framework = 'TokenInsights Desktop.app/Contents/Frameworks/Bundled Framework.framework'
    f.archive('desktop', 'v1', 'darwin', (root) => {
      const version = join(root, framework, 'Versions/A')
      mkdirSync(join(version, 'Resources'), { recursive: true })
      writeFileSync(join(version, 'Bundled Framework'), 'framework-binary')
      writeFileSync(join(version, 'Resources/Info.plist'), 'framework-metadata')
      symlinkSync('A', join(root, framework, 'Versions/Current'))
      symlinkSync('Versions/Current/Bundled Framework', join(root, framework, 'Bundled Framework'))
      symlinkSync('Versions/Current/Resources', join(root, framework, 'Resources'))
      linkSync(join(version, 'Bundled Framework'), join(version, 'Framework hardlink'))
    })
    executable(join(f.bin, 'tar'), `exec ${tar} "$@"`)
    const result = f.run('install.sh', [], { TEST_OS: 'Darwin' })
    assert.equal(result.status, 0, result.stderr)
    const installed = join(f.installed('desktop'), framework)
    assert.equal(readlinkSync(join(installed, 'Versions/Current')), 'A')
    assert.equal(readFileSync(join(installed, 'Bundled Framework'), 'utf8'), 'framework-binary')
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
    assert.equal(f.runCli().status, 0)
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
    const result = f.runCli()
    assert.notEqual(result.status, 0)
    assert.match(readFileSync(join(f.installed('cli'), 'tokeninsights'), 'utf8'), /echo v1/)
  })
}
void test('CLI-only stable and main installers replace a whole product including backend', (t) => {
  const f = fixture(t)
  f.archive()
  const first = f.runCli()
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
  const next = f.runCli('install-main.sh')
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
for (const { script, channel, source } of [
  { script: 'install.sh', channel: 'stable', source: /latest\/download\/tokeninsights-desktop/ },
  { script: 'install-main.sh', channel: 'main', source: /main-build\/tokeninsights-desktop/ },
]) {
  void test(`${script} installs desktop and its CLI by default`, (t) => {
    const f = fixture(t)
    f.archive('desktop')
    const result = f.run(script)
    assert.equal(result.status, 0, result.stderr)
    assert.equal(readFileSync(join(f.installed('desktop'), '.channel'), 'utf8'), `${channel}\n`)
    assert.equal(
      execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
      'cli-v1',
    )
    assert.equal(
      execFileSync(join(f.home, '.local/bin/tokeninsights-desktop'), { encoding: 'utf8' }).trim(),
      'v1',
    )
    assert.ok(!existsSync(f.installed('cli')))
    assert.match(readFileSync(f.log, 'utf8'), source)
    assert.doesNotMatch(readFileSync(f.log, 'utf8'), /tokeninsights-cli/)
  })
}
void test('--cli-only leaves no desktop product, launcher or entry', (t) => {
  const f = fixture(t)
  f.archive()
  const result = f.runCli()
  assert.equal(result.status, 0, result.stderr)
  assert.equal(
    execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
    'v1',
  )
  assert.ok(!existsSync(f.installed('desktop')))
  assert.ok(!existsSync(join(f.home, '.local/bin/tokeninsights-desktop')))
  assert.ok(!existsSync(join(f.home, '.local/share/applications/tokeninsights-desktop.desktop')))
  assert.doesNotMatch(readFileSync(f.log, 'utf8'), /tokeninsights-desktop/)
})
void test('desktop default supports main and registers its desktop entry', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  const result = f.run('install.sh', ['--channel=main'])
  assert.equal(result.status, 0, result.stderr)
  assert.equal(readFileSync(join(f.installed('desktop'), '.channel'), 'utf8'), 'main\n')
  const entry = readFileSync(
    join(f.home, '.local/share/applications/tokeninsights-desktop.desktop'),
    'utf8',
  )
  assert.ok(entry.includes(`Icon=${join(f.installed('desktop'), 'tokeninsights-desktop.png')}\n`))
})
void test('upgrades retain the valid launcher without a remove-and-republish gap', (t) => {
  const f = fixture(t)
  f.archive()
  assert.equal(f.runCli().status, 0)
  f.archive('cli', 'v2')
  executable(
    join(f.bin, 'mv'),
    'case "$1" in *.tokeninsights-link-*) exit 17 ;; *) exec /bin/mv "$@" ;; esac',
  )
  const result = f.runCli()
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
    assert.equal(f.runCli().status, 0)
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
    const result = f.runCli()
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
  assert.match(f.runCli().stderr, /unowned path/)
  rmSync(f.installed('cli'), { recursive: true })
  mkdirSync(join(f.home, '.local/bin'), { recursive: true })
  executable(join(f.home, '.local/bin/tokeninsights'), 'echo foreign')
  assert.match(f.runCli().stderr, /unowned path/)
})
void test('macOS desktop bundle supports replacement', (t) => {
  const f = fixture(t)
  f.archive('desktop', 'v1', 'darwin')
  const result = f.run('install.sh', [], { TEST_OS: 'Darwin' })
  assert.equal(result.status, 0, result.stderr)
  f.archive('desktop', 'v2', 'darwin')
  const upgrade = f.run('install.sh', [], { TEST_OS: 'Darwin' })
  assert.equal(upgrade.status, 0, upgrade.stderr)
  assert.ok(existsSync(join(f.home, 'Applications/TokenInsights Desktop.app')))
})
for (const os of ['linux', 'darwin']) {
  void test(`${os} desktop requires its bundled CLI before installation`, (t) => {
    const f = fixture(t)
    const platform = os === 'darwin' ? 'darwin' : 'linux'
    f.archive('desktop', 'v1', platform, (root) =>
      rmSync(
        join(
          root,
          platform === 'darwin'
            ? 'TokenInsights Desktop.app/Contents/MacOS/tokeninsights'
            : 'tokeninsights',
        ),
      ),
    )
    const rejected = f.run('install.sh', [], os === 'darwin' ? { TEST_OS: 'Darwin' } : {})
    assert.notEqual(rejected.status, 0)
    assert.match(rejected.stderr, /archive lacks executable tokeninsights/)
    assert.ok(!existsSync(f.installed('desktop')))
  })
  void test(`${os} latest CLI or desktop install owns the command`, (t) => {
    const f = fixture(t)
    const platform = os === 'darwin' ? 'darwin' : 'linux'
    const overrides = os === 'darwin' ? { TEST_OS: 'Darwin' } : {}
    const command = join(f.home, '.local/bin/tokeninsights')
    f.archive('desktop', 'v1', platform)
    const installed = f.run('install.sh', [], overrides)
    assert.equal(installed.status, 0, installed.stderr)
    assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'cli-v1')
    const bundled = readlinkSync(command)
    f.archive('cli', 'cli-only', platform)
    const standalone = f.runCli('install.sh', [], overrides)
    assert.equal(standalone.status, 0, standalone.stderr)
    assert.equal(readlinkSync(command), join(f.installed('cli'), 'tokeninsights'))
    assert.notEqual(readlinkSync(command), bundled)
    f.archive('desktop', 'v2', platform)
    const upgrade = f.run('install.sh', [], overrides)
    assert.equal(upgrade.status, 0, upgrade.stderr)
    assert.match(upgrade.stdout, /now runs the desktop CLI/)
    assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'cli-v2')
    assert.ok(existsSync(join(f.installed('cli'), 'tokeninsights')))
    const steady = f.run('install.sh', [], overrides)
    assert.equal(steady.status, 0, steady.stderr)
    assert.doesNotMatch(steady.stdout, /now runs the desktop CLI/)
  })
  void test(`${os} desktop rejects an unrelated command before replacing its product`, (t) => {
    const f = fixture(t)
    const platform = os === 'darwin' ? 'darwin' : 'linux'
    const overrides = os === 'darwin' ? { TEST_OS: 'Darwin' } : {}
    f.archive('desktop', 'v1', platform)
    assert.equal(f.run('install.sh', [], overrides).status, 0)
    const command = join(f.home, '.local/bin/tokeninsights')
    rmSync(command)
    executable(command, 'echo unrelated')
    f.archive('desktop', 'v2', platform)
    const rejected = f.run('install.sh', [], overrides)
    assert.notEqual(rejected.status, 0)
    assert.match(rejected.stderr, /unowned path/)
    assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'unrelated')
    const binary =
      platform === 'darwin'
        ? join(f.installed('desktop'), 'TokenInsights Desktop.app/Contents/MacOS/tokeninsights')
        : join(f.installed('desktop'), 'tokeninsights')
    assert.equal(execFileSync(binary, { encoding: 'utf8' }).trim(), 'cli-v1')
  })
}
for (const platform of ['linux', 'darwin']) {
  const os = platform === 'darwin' ? 'darwin' : 'linux'
  const overrides = os === 'darwin' ? { TEST_OS: 'Darwin' } : {}
  for (const product of ['cli', 'desktop']) {
    const current = product === 'desktop' ? 'desktop' : 'cli'
    const script = 'install.sh'
    const productArgs = current === 'cli' ? ['--cli-only'] : []
    void test(`${os} ${product} takes command ownership from the renamed MyGo product`, (t) => {
      const f = fixture(t)
      const retired = f.retiredMyGo(os)
      const launcherTarget = readlinkSync(retired.launcher)
      f.archive(current, 'v2', os)
      const result = f.run(
        script,
        [...productArgs, ...(os === 'linux' ? ['--channel=main'] : [])],
        overrides,
      )
      assert.equal(result.status, 0, result.stderr)
      assert.equal(
        execFileSync(retired.command, { encoding: 'utf8' }).trim(),
        current === 'desktop' ? 'cli-v2' : 'v2',
      )
      assert.equal(readFileSync(retired.marker, 'utf8'), 'tokeninsights-product:mygo\n')
      assert.equal(execFileSync(retired.cli, { encoding: 'utf8' }).trim(), 'retired-cli')
      assert.equal(readlinkSync(retired.launcher), launcherTarget)
      const steady = f.run(script, productArgs, overrides)
      assert.equal(steady.status, 0, steady.stderr)
    })
    void test(`${os} failed ${product} handover restores the MyGo command`, (t) => {
      const f = fixture(t)
      const retired = f.retiredMyGo(os)
      f.archive(current, 'v2', os)
      executable(
        join(f.bin, 'rmdir'),
        'case "$1" in *.tokeninsights-link-*) if [ "$(readlink "$HOME/.local/bin/tokeninsights")" != "$TEST_RETIRED_CLI" ]; then echo injected-publication-failure >&2; exit 17; fi ;; esac\nexec /bin/rmdir "$@"',
      )
      const result = f.run(script, productArgs, { ...overrides, TEST_RETIRED_CLI: retired.cli })
      assert.notEqual(result.status, 0)
      assert.match(result.stderr, /injected-publication-failure/)
      assert.equal(readlinkSync(retired.command), retired.cli)
      assert.equal(execFileSync(retired.command, { encoding: 'utf8' }).trim(), 'retired-cli')
      assert.ok(!existsSync(f.installed(current)))
    })
    for (const ownership of ['missing marker', 'wrong marker', 'symlinked product']) {
      void test(`${os} ${product} rejects a MyGo command with ${ownership}`, (t) => {
        const f = fixture(t)
        const retired = f.retiredMyGo(os)
        if (ownership === 'missing marker') rmSync(retired.marker)
        if (ownership === 'wrong marker')
          writeFileSync(retired.marker, 'tokeninsights-product:desktop\n')
        if (ownership === 'symlinked product') {
          const foreign = join(f.root, 'foreign')
          execFileSync('mv', [retired.product, foreign])
          symlinkSync(foreign, retired.product)
        }
        f.archive(current, 'v2', os)
        const result = f.run(script, productArgs, overrides)
        assert.notEqual(result.status, 0)
        assert.match(result.stderr, /refusing to replace unowned path/)
        assert.equal(readlinkSync(retired.command), retired.cli)
        assert.equal(execFileSync(retired.command, { encoding: 'utf8' }).trim(), 'retired-cli')
        assert.ok(!existsSync(f.installed(current)))
      })
    }
  }
}
void test('failed CLI command handover restores owned desktop command and product state', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  assert.equal(f.run('install.sh').status, 0)
  const command = join(f.home, '.local/bin/tokeninsights')
  const original = readlinkSync(command)
  f.archive('cli')
  executable(
    join(f.bin, 'rmdir'),
    'case "$1" in *.tokeninsights-link-*) exit 17 ;; *) exec /bin/rmdir "$@" ;; esac',
  )
  const result = f.runCli()
  assert.notEqual(result.status, 0)
  assert.equal(readlinkSync(command), original)
  assert.ok(!existsSync(f.installed('cli')))
  assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'cli-v1')
})
void test('failed desktop install restores the CLI-only command', (t) => {
  const f = fixture(t)
  f.archive('cli', 'cli-only')
  assert.equal(f.runCli().status, 0)
  const command = join(f.home, '.local/bin/tokeninsights')
  const original = readlinkSync(command)
  f.archive('desktop')
  executable(
    join(f.bin, 'cp'),
    'case "$*" in *desktop-entry*) exit 17 ;; *) exec /bin/cp "$@" ;; esac',
  )
  const result = f.run('install.sh')
  assert.notEqual(result.status, 0)
  assert.equal(readlinkSync(command), original)
  assert.ok(!existsSync(f.installed('desktop')))
  assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'cli-only')
})
void test('installers warn only when PATH does not run the published command', (t) => {
  const f = fixture(t)
  const local = join(f.home, '.local/bin')
  const system = `${f.bin}:/usr/bin:/bin`
  f.archive('desktop')
  assert.equal(
    f.run('install.sh', [], { PATH: `${local}:${system}` }).stderr.includes('warning: PATH'),
    false,
  )
  assert.match(
    f.run('install.sh', ['--no-modify-path'], { PATH: system }).stderr,
    /warning: PATH runs nothing/,
  )
  executable(join(f.bin, 'tokeninsights'), 'echo retired')
  const shadowed = f.run('install.sh', [], { PATH: `${system}:${local}` })
  assert.equal(shadowed.status, 0, shadowed.stderr)
  assert.match(shadowed.stderr, /warning: PATH runs .*\/bin\/tokeninsights instead of/)
})
const profileMarker = '# Added by the tokeninsights installer'
const occurrences = (path: string) =>
  readFileSync(path, 'utf8')
    .split('\n')
    .filter((line) => line === profileMarker).length
for (const { shell, profiles } of [
  { shell: '/bin/zsh', profiles: ['.zshenv'] },
  { shell: '/bin/bash', profiles: ['.bashrc', '.profile'] },
  { shell: '/bin/dash', profiles: ['.profile'] },
]) {
  void test(`${shell} profiles put the CLI on PATH once, keeping existing content`, (t) => {
    const f = fixture(t)
    f.archive('desktop')
    for (const profile of profiles) writeFileSync(join(f.home, profile), '# mine\n')
    for (const attempt of [1, 2]) {
      const result = f.run('install.sh', [], { SHELL: shell })
      assert.equal(result.status, 0, `attempt ${attempt}: ${result.stderr}`)
      assert.match(result.stdout, /For this shell, run: export PATH=/)
      assert.doesNotMatch(result.stderr, /warning: PATH/)
    }
    for (const profile of profiles) {
      const path = join(f.home, profile)
      assert.equal(occurrences(path), 1)
      assert.match(readFileSync(path, 'utf8'), /^# mine\n/)
      const found = execFileSync('sh', ['-c', '. "$1"; command -v tokeninsights', 'sh', path], {
        encoding: 'utf8',
        env: { HOME: f.home, PATH: '/usr/bin:/bin' },
      })
      assert.equal(found.trim(), join(f.home, '.local/bin/tokeninsights'))
    }
  })
}
void test('fish profile lives in its own conf.d file', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  for (const attempt of [1, 2]) {
    const result = f.run('install.sh', [], { SHELL: '/usr/bin/fish' })
    assert.equal(result.status, 0, `attempt ${attempt}: ${result.stderr}`)
    assert.match(result.stdout, /For this shell, run: set -gx PATH/)
  }
  const profile = join(f.home, '.config/fish/conf.d/tokeninsights.fish')
  assert.equal(occurrences(profile), 1)
  assert.match(readFileSync(profile, 'utf8'), /or set -gx PATH "\$HOME\/\.local\/bin" \$PATH/)
  assert.ok(!existsSync(join(f.home, '.profile')))
})
void test('bash prefers an existing login profile; zsh honors ZDOTDIR', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  writeFileSync(join(f.home, '.bash_profile'), '')
  assert.equal(f.run('install.sh', [], { SHELL: '/bin/bash' }).status, 0)
  assert.equal(occurrences(join(f.home, '.bash_profile')), 1)
  assert.ok(!existsSync(join(f.home, '.profile')))
  const zdotdir = join(f.root, 'zsh config')
  assert.equal(f.run('install.sh', [], { SHELL: '/bin/zsh', ZDOTDIR: zdotdir }).status, 0)
  assert.equal(occurrences(join(zdotdir, '.zshenv')), 1)
  assert.ok(!existsSync(join(f.home, '.zshenv')))
})
void test('CLI-only install also puts the CLI on PATH', (t) => {
  const f = fixture(t)
  f.archive()
  assert.equal(f.runCli().status, 0)
  assert.equal(occurrences(join(f.home, '.profile')), 1)
})
void test('profiles stay untouched when PATH already works, on opt-out and on failed install', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  const profile = join(f.home, '.profile')
  const onPath = f.run('install.sh', [], {
    PATH: `${join(f.home, '.local/bin')}:${f.bin}:/usr/bin:/bin`,
  })
  assert.equal(onPath.status, 0, onPath.stderr)
  assert.doesNotMatch(onPath.stdout, /For this shell/)
  assert.ok(!existsSync(profile))
  const optOut = f.run('install.sh', ['--no-modify-path'])
  assert.equal(optOut.status, 0, optOut.stderr)
  assert.doesNotMatch(optOut.stdout, /For this shell/)
  assert.ok(!existsSync(profile))
  writeFileSync(join(f.release, 'checksums.txt'), '')
  assert.notEqual(f.run().status, 0)
  assert.ok(!existsSync(profile))
})
void test('an unwritable profile warns and keeps the completed install', (t) => {
  const f = fixture(t)
  f.archive('desktop')
  // A directory rejects writes even when validation runs as root.
  mkdirSync(join(f.home, '.profile'))
  const result = f.run()
  assert.equal(result.status, 0, result.stderr)
  assert.match(result.stderr, /warning: could not add ~\/\.local\/bin to your shell profile/)
  assert.match(result.stderr, /warning: PATH runs nothing/)
  assert.equal(
    execFileSync(join(f.home, '.local/bin/tokeninsights'), { encoding: 'utf8' }).trim(),
    'cli-v1',
  )
})
void test('rejects unsupported host and channel before download', (t) => {
  const f = fixture(t)
  assert.match(f.run('install.sh', ['--product=cli']).stderr, /unknown option/)
  assert.match(f.run('install.sh', [], { TEST_OS: 'FreeBSD' }).stderr, /unsupported OS/)
  assert.match(f.run('install.sh', [], { TEST_ARCH: 'i686' }).stderr, /unsupported architecture/)
  assert.match(f.run('install.sh', ['--channel=nightly']).stderr, /unknown option/)
  for (const args of [
    ['--channel=edge', '--cli-only'],
    ['--cli-only', '--channel=edge'],
  ])
    assert.match(f.run('install.sh', args).stderr, /cannot be combined with --cli-only/)
  assert.match(f.runCli('install-edge.sh').stderr, /cannot be combined with --cli-only/)
  assert.ok(!existsSync(f.log))
})
for (const script of ['install-edge.sh', 'install.sh']) {
  void test(`${script} edge installs only desktop-native with its own scheme`, (t) => {
    const f = fixture(t)
    f.archive('desktop-native')
    const result = f.run(script, script === 'install.sh' ? ['--channel=edge'] : [])
    assert.equal(result.status, 0, result.stderr)
    const product = f.installed('desktop-native')
    assert.equal(readFileSync(join(product, '.channel'), 'utf8'), 'edge\n')
    assert.equal(
      readFileSync(join(product, '.installed-by-tokeninsights'), 'utf8'),
      'tokeninsights-product:desktop-native\n',
    )
    assert.ok(existsSync(join(product, 'tokeninsights-backend')))
    assert.equal(
      execFileSync(join(f.home, '.local/bin/tokeninsights-native'), { encoding: 'utf8' }).trim(),
      'v1',
    )
    const entry = readFileSync(
      join(f.home, '.local/share/applications/tokeninsights-desktop-native.desktop'),
      'utf8',
    )
    assert.ok(entry.includes('Name=TokenInsights Native\n'))
    assert.ok(entry.includes(`Exec="${join(product, 'tokeninsights-native')}" %u\n`))
    assert.ok(entry.includes(`Icon=${join(product, 'tokeninsights-native.png')}\n`))
    assert.ok(entry.includes('MimeType=x-scheme-handler/tokeninsights-native;\n'))
    assert.ok(entry.includes('X-TokenInsights-Product=desktop-native\n'))
    assert.doesNotMatch(entry, /x-scheme-handler\/tokeninsights(?:-desktop)?;/)
    // The native experiment never owns the command or edits shell profiles.
    assert.ok(!existsSync(join(f.home, '.local/bin/tokeninsights')))
    assert.ok(!existsSync(join(f.home, '.profile')))
    assert.ok(!existsSync(f.installed('desktop')) && !existsSync(f.installed('cli')))
    const urls = readFileSync(f.log, 'utf8')
    assert.match(urls, /releases\/download\/edge-build\/tokeninsights-desktop-native_linux_amd64/)
    assert.doesNotMatch(urls, /tokeninsights-(?:cli|desktop)_/)
  })
}
void test('edge registers the native scheme with xdg-mime only', (t) => {
  const f = fixture(t)
  const mime = join(f.root, 'mime')
  executable(join(f.bin, 'xdg-mime'), `printf '%s\\n' "$*" >> ${JSON.stringify(mime)}`)
  f.archive('desktop-native')
  const result = f.run('install-edge.sh')
  assert.equal(result.status, 0, result.stderr)
  assert.equal(
    readFileSync(mime, 'utf8'),
    'default tokeninsights-desktop-native.desktop x-scheme-handler/tokeninsights-native\n',
  )
})
for (const os of ['linux', 'darwin']) {
  void test(`${os} edge coexists with desktop and leaves its command untouched`, (t) => {
    const f = fixture(t)
    const platform = os === 'darwin' ? 'darwin' : 'linux'
    const overrides = { TEST_OS: os === 'darwin' ? 'Darwin' : 'Linux', TEST_ARCH: 'arm64' }
    const command = join(f.home, '.local/bin/tokeninsights')
    // Edge has no macOS Intel build, so both products use arm64.
    const archive = (product: Product) => f.archive(product, 'v1', platform, undefined, 'arm64')
    archive('desktop')
    const desktop = f.run('install.sh', [], overrides)
    assert.equal(desktop.status, 0, desktop.stderr)
    const owned = readlinkSync(command)
    archive('desktop-native')
    const edge = f.run('install-edge.sh', [], overrides)
    assert.equal(edge.status, 0, edge.stderr)
    assert.equal(readlinkSync(command), owned)
    assert.equal(execFileSync(command, { encoding: 'utf8' }).trim(), 'cli-v1')
    assert.equal(readFileSync(join(f.installed('desktop'), '.channel'), 'utf8'), 'stable\n')
    assert.equal(readFileSync(join(f.installed('desktop-native'), '.channel'), 'utf8'), 'edge\n')
    if (os === 'darwin') {
      assert.ok(existsSync(join(f.home, 'Applications/TokenInsights Desktop.app')))
      assert.equal(
        readlinkSync(join(f.home, 'Applications/TokenInsights Native.app')),
        join(f.installed('desktop-native'), 'TokenInsights Native.app'),
      )
      assert.ok(!existsSync(join(f.home, '.local/bin/tokeninsights-native')))
    } else {
      const entries = join(f.home, '.local/share/applications')
      assert.ok(existsSync(join(entries, 'tokeninsights-desktop.desktop')))
      assert.ok(existsSync(join(entries, 'tokeninsights-desktop-native.desktop')))
    }
  })
}
void test('edge refuses an unowned tokeninsights-native launcher', (t) => {
  const f = fixture(t)
  f.archive('desktop-native')
  mkdirSync(join(f.home, '.local/bin'), { recursive: true })
  executable(join(f.home, '.local/bin/tokeninsights-native'), 'echo foreign')
  const result = f.run('install-edge.sh')
  assert.match(result.stderr, /unowned path/)
  assert.ok(!existsSync(f.installed('desktop-native')))
})
void test('main channel rejects macOS Intel before download; Apple Silicon and stable proceed', (t) => {
  const f = fixture(t)
  const intel = { TEST_OS: 'Darwin' }
  assert.match(f.run('install-main.sh', [], intel).stderr, /no macOS Intel build/)
  assert.match(f.run('install.sh', ['--channel=main'], intel).stderr, /no macOS Intel build/)
  assert.match(f.run('install-edge.sh', [], intel).stderr, /edge channel has no macOS Intel build/)
  assert.ok(!existsSync(f.log))
  f.run('install-main.sh', [], { ...intel, TEST_ARCH: 'arm64' })
  assert.match(
    readFileSync(f.log, 'utf8'),
    /main-build\/tokeninsights-desktop_darwin_arm64\.tar\.gz/,
  )
  f.run('install.sh', [], intel)
  assert.match(
    readFileSync(f.log, 'utf8'),
    /latest\/download\/tokeninsights-desktop_darwin_amd64\.tar\.gz/,
  )
})
