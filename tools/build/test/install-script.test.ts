import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync, spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { workspacePath } from '../src/workspace.ts'

const script = workspacePath('install.sh')

interface Fixture {
  root: string
  home: string
  release: string
  log: string
  run: (uname?: { system: string; machine: string }) => ReturnType<typeof spawnSync>
}

function executable(path: string, body: string) {
  writeFileSync(path, `#!/bin/sh\n${body}\n`)
  chmodSync(path, 0o755)
}

function checksum(path: string, name: string): string {
  return `${createHash('sha256').update(readFileSync(path)).digest('hex')}  ${name}\n`
}

// Builds a release directory served through file:// and PATH stubs for host tools.
function fixture(): Fixture {
  const root = mkdtempSync(join(tmpdir(), 'tokeninsights-install-'))
  const home = join(root, 'home')
  const release = join(root, 'release')
  const stubs = join(root, 'bin')
  const log = join(root, 'calls.log')
  for (const dir of [home, release, stubs]) mkdirSync(dir)
  writeFileSync(log, '')
  // Records the default handler like real xdg-mime, which needs an existing config dir.
  executable(
    join(stubs, 'xdg-mime'),
    `echo "xdg-mime $*" >> "${log}"
config="\${XDG_CONFIG_HOME:-$HOME/.config}"
case "$1" in
default) [ -d "$config" ] && echo "$2" > "$config/handler" ;;
query) cat "$config/handler" 2>/dev/null ;;
esac`,
  )
  executable(
    join(stubs, 'update-desktop-database'),
    `echo "update-desktop-database $*" >> "${log}"`,
  )
  executable(join(stubs, 'sysctl'), 'echo 1')
  const run = (uname = { system: 'Linux', machine: 'x86_64' }) => {
    executable(
      join(stubs, 'uname'),
      `case "$1" in -s) echo ${uname.system} ;; -m) echo ${uname.machine} ;; esac`,
    )
    return spawnSync('sh', [script], {
      encoding: 'utf8',
      env: {
        PATH: `${stubs}:/usr/bin:/bin`,
        HOME: home,
        TMPDIR: root,
        TOKENINSIGHTS_DOWNLOAD_URL: `file://${release}`,
      },
    })
  }
  return { root, home, release, log, run }
}

function linuxArchive(f: Fixture, binary: string, arch = 'amd64'): string {
  const name = `tokeninsights-desktop_linux_${arch}`
  const dir = join(f.root, 'stage', name)
  mkdirSync(dir, { recursive: true })
  executable(join(dir, 'tokeninsights-desktop'), binary)
  writeFileSync(join(dir, 'tokeninsights-desktop.png'), 'png')
  writeFileSync(join(dir, 'LICENSE'), 'MIT')
  const archive = join(f.release, `${name}.tar.gz`)
  execFileSync('tar', ['-C', join(f.root, 'stage'), '-czf', archive, name])
  return archive
}

function darwinArchive(f: Fixture): string {
  const name = 'tokeninsights-desktop_darwin_arm64'
  const contents = join(f.root, 'stage', name, 'TokenInsights.app', 'Contents')
  mkdirSync(join(contents, 'MacOS'), { recursive: true })
  executable(join(contents, 'MacOS', 'TokenInsights'), 'echo app')
  writeFileSync(join(contents, 'Info.plist'), '<string>com.flexdinesh.tokeninsights</string>')
  const archive = join(f.release, `${name}.tar.gz`)
  execFileSync('tar', ['-C', join(f.root, 'stage'), '-czf', archive, name])
  return archive
}

function publish(f: Fixture, archives: string[], extra = '') {
  const unrelated = 'a'.repeat(64) + '  tokeninsights_0.1.0_linux_amd64.tar.gz\n'
  writeFileSync(
    join(f.release, 'checksums.txt'),
    unrelated +
      archives.map((archive) => checksum(archive, archive.split('/').at(-1) ?? '')).join('') +
      extra,
  )
}

void test('installs the Linux app, desktop entry and scheme handler, then upgrades in place', () => {
  const f = fixture()
  publish(f, [linuxArchive(f, 'echo v1')])
  const first = f.run()
  assert.equal(first.status, 0, String(first.stderr))
  const data = join(f.home, '.local', 'share')
  const binary = join(data, 'tokeninsights-desktop', 'tokeninsights-desktop')
  assert.equal(statSync(binary).mode & 0o777, 0o755)
  assert.ok(existsSync(join(data, 'tokeninsights-desktop', 'tokeninsights-desktop.png')))
  const entry = readFileSync(join(data, 'applications', 'tokeninsights-desktop.desktop'), 'utf8')
  assert.match(entry, new RegExp(`^Exec="${binary}" %u$`, 'm'))
  assert.match(entry, /^MimeType=x-scheme-handler\/tokeninsights;$/m)
  assert.match(entry, /^Name=TokenInsights$/m)
  const calls = readFileSync(f.log, 'utf8')
  assert.match(
    calls,
    /^xdg-mime default tokeninsights-desktop\.desktop x-scheme-handler\/tokeninsights$/m,
  )
  assert.match(calls, /^update-desktop-database .*applications$/m)
  // A fresh home has no ~/.config; registration must still succeed.
  assert.equal(
    readFileSync(join(f.home, '.config', 'handler'), 'utf8'),
    'tokeninsights-desktop.desktop\n',
  )
  assert.doesNotMatch(String(first.stderr), /not registered/)
  assert.match(String(first.stdout), /Run tokeninsights web/)

  publish(f, [linuxArchive(f, 'echo v2')])
  const second = f.run()
  assert.equal(second.status, 0, String(second.stderr))
  assert.match(readFileSync(binary, 'utf8'), /echo v2/)
})

function installedApp(f: Fixture): string {
  return join(f.home, '.local', 'share', 'tokeninsights-desktop')
}

void test('rejects checksum mismatches, missing entries and unsupported hosts without installing', () => {
  const mismatch = fixture()
  const archive = linuxArchive(mismatch, 'echo v1')
  writeFileSync(
    join(mismatch.release, 'checksums.txt'),
    `${'0'.repeat(64)}  tokeninsights-desktop_linux_amd64.tar.gz\n`,
  )
  let result = mismatch.run()
  assert.notEqual(result.status, 0)
  assert.match(String(result.stderr), /checksum mismatch/)
  assert.ok(!existsSync(installedApp(mismatch)))

  const missing = fixture()
  linuxArchive(missing, 'echo v1')
  publish(missing, [])
  result = missing.run()
  assert.notEqual(result.status, 0)
  assert.match(String(result.stderr), /no entry for tokeninsights-desktop_linux_amd64/)
  assert.ok(!existsSync(installedApp(missing)))

  const duplicate = fixture()
  const duplicated = linuxArchive(duplicate, 'echo v1')
  publish(duplicate, [duplicated], checksum(duplicated, 'tokeninsights-desktop_linux_amd64.tar.gz'))
  result = duplicate.run()
  assert.notEqual(result.status, 0)
  assert.match(String(result.stderr), /duplicate entries/)

  const unsupported = fixture()
  publish(unsupported, [archive])
  result = unsupported.run({ system: 'Linux', machine: 'i686' })
  assert.notEqual(result.status, 0)
  assert.match(String(result.stderr), /unsupported architecture i686/)
  result = unsupported.run({ system: 'FreeBSD', machine: 'x86_64' })
  assert.notEqual(result.status, 0)
  assert.match(String(result.stderr), /unsupported OS FreeBSD/)
})

void test('installs the macOS bundle into ~/Applications and refuses to replace another app', () => {
  const f = fixture()
  publish(f, [darwinArchive(f)])
  const result = f.run({ system: 'Darwin', machine: 'x86_64' })
  assert.equal(result.status, 0, String(result.stderr))
  const app = join(f.home, 'Applications', 'TokenInsights.app')
  assert.ok(existsSync(join(app, 'Contents', 'MacOS', 'TokenInsights')))

  const other = fixture()
  publish(other, [darwinArchive(other)])
  const foreign = join(other.home, 'Applications', 'TokenInsights.app', 'Contents')
  mkdirSync(foreign, { recursive: true })
  writeFileSync(join(foreign, 'Info.plist'), '<string>com.example.other</string>')
  const refused = other.run({ system: 'Darwin', machine: 'arm64' })
  assert.notEqual(refused.status, 0)
  assert.match(String(refused.stderr), /is not TokenInsights/)
  assert.equal(
    readFileSync(join(foreign, 'Info.plist'), 'utf8'),
    '<string>com.example.other</string>',
  )
})
