import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { chmodSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { archiveName, parseTarget, stagePackage, writeArchive } from '../src/build-desktop.ts'

function scratch(): string {
  return mkdtempSync(join(tmpdir(), 'tokeninsights-build-desktop-'))
}

function linuxBuild(root: string): string {
  const build = join(root, 'build', 'linux-amd64')
  mkdirSync(build, { recursive: true })
  writeFileSync(join(build, 'tokeninsights-desktop'), 'binary')
  chmodSync(join(build, 'tokeninsights-desktop'), 0o755)
  writeFileSync(join(build, 'tokeninsights-desktop-512x512.png'), 'png')
  writeFileSync(join(build, 'tokeninsights-desktop.deb'), 'ignored')
  return build
}

function darwinBuild(root: string, plist: string): string {
  const build = join(root, 'build', 'darwin-arm64')
  const contents = join(build, 'TokenInsights.app', 'Contents')
  mkdirSync(join(contents, 'MacOS'), { recursive: true })
  writeFileSync(join(contents, 'MacOS', 'TokenInsights'), 'binary')
  chmodSync(join(contents, 'MacOS', 'TokenInsights'), 0o755)
  writeFileSync(join(contents, 'Info.plist'), plist)
  return build
}

void test('archive names are unversioned per OS and architecture', () => {
  assert.equal(archiveName(parseTarget('linux/amd64')), 'tokeninsights-desktop_linux_amd64.tar.gz')
  assert.equal(
    archiveName(parseTarget('darwin/arm64')),
    'tokeninsights-desktop_darwin_arm64.tar.gz',
  )
  for (const value of ['windows/amd64', 'linux/386', 'linux', 'darwin/arm64/x'])
    assert.throws(() => parseTarget(value))
})

void test('packages the Linux executable, icon and license for install.sh', async () => {
  const root = scratch()
  const target = parseTarget('linux/amd64')
  const stage = join(root, 'stage')
  await stagePackage(linuxBuild(root), target, stage)
  const out = join(root, 'out')
  mkdirSync(out)
  const archive = writeArchive(stage, target, out)
  const entries = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' })
    .trim()
    .split('\n')
    .toSorted()
  assert.deepEqual(entries, [
    'tokeninsights-desktop_linux_amd64/',
    'tokeninsights-desktop_linux_amd64/LICENSE',
    'tokeninsights-desktop_linux_amd64/tokeninsights-desktop',
    'tokeninsights-desktop_linux_amd64/tokeninsights-desktop.png',
  ])
})

void test('macOS packages require the bundle identifier and URL scheme', async () => {
  const root = scratch()
  const target = parseTarget('darwin/arm64')
  const valid =
    '<string>com.flexdinesh.tokeninsights</string><array><string>tokeninsights</string></array>'
  await stagePackage(darwinBuild(root, valid), target, join(root, 'stage'))

  const other = scratch()
  await assert.rejects(
    stagePackage(
      darwinBuild(other, '<string>com.flexdinesh.tokeninsights</string>'),
      target,
      join(other, 'stage'),
    ),
    /lacks tokeninsights/,
  )
})
