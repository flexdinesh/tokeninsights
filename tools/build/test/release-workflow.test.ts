import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { embeddedAssets, prepareRelease } from '../src/install-refs.ts'
import { workspacePath } from '../src/workspace.ts'

function stepScript(name: string): string {
  const lines = readFileSync(workspacePath('.github/workflows/release.yml'), 'utf8').split('\n')
  const step = lines.findIndex((line) => line === `      - name: ${name}`)
  assert.ok(step >= 0, `missing workflow step ${name}`)
  const run = lines.findIndex((line, index) => index > step && line === '        run: |')
  assert.ok(run > step, `missing script for ${name}`)
  const script: string[] = []
  for (const line of lines.slice(run + 1)) {
    if (line !== '' && !line.startsWith('          ')) break
    script.push(line.slice(10))
  }
  assert.ok(script.length > 0)
  return script.join('\n')
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'tokeninsights-release-workflow-'))
  const checkout = join(root, 'checkout')
  const remote = join(root, 'remote.git')
  const env = { ...process.env }
  for (const name of execFileSync('git', ['rev-parse', '--local-env-vars'], { encoding: 'utf8' })
    .trim()
    .split('\n'))
    delete env[name]
  env.GIT_CONFIG_GLOBAL = join(root, 'gitconfig')
  env.GIT_CONFIG_NOSYSTEM = '1'
  env.TMPDIR = join(root, 'tmp')
  mkdirSync(checkout)
  mkdirSync(env.TMPDIR)
  writeFileSync(env.GIT_CONFIG_GLOBAL, '')
  const git = (...args: string[]) =>
    execFileSync('git', args, { cwd: checkout, env, encoding: 'utf8', stdio: 'pipe' }).trim()
  git('init', '--bare', '--initial-branch=main', remote)
  git('init', '--initial-branch=main')
  git('config', 'user.name', 'Release fixture')
  git('config', 'user.email', 'fixture@example.invalid')
  git('config', 'commit.gpgsign', 'false')
  git('config', 'tag.gpgsign', 'false')
  git('config', 'core.hooksPath', join(root, 'no-hooks'))
  git('remote', 'add', 'origin', remote)
  writeFileSync(join(checkout, '.gitignore'), `${embeddedAssets}/\n`)
  writeFileSync(join(checkout, 'source.txt'), 'historical source without publication tooling\n')
  git('add', '.')
  git('commit', '-m', 'historical source')
  git('push', 'origin', 'main')
  const execute = (step: string, settings: NodeJS.ProcessEnv) =>
    spawnSync('bash', ['-c', stepScript(step)], {
      cwd: checkout,
      env: { ...env, ...settings },
      encoding: 'utf8',
    })
  return { root, checkout, env, git, execute }
}

function checksum(name: string, bytes: string): string {
  return `${createHash('sha256').update(bytes).digest('hex')}  ${name}\n`
}

function publisher(version = '0.1.0') {
  const f = fixture()
  f.git('tag', 'packages/cli/v0.1.0')
  f.git('tag', 'packages/cli/v0.2.0')
  // Unrelated and prerelease tags must not control stable latest metadata.
  f.git('tag', 'plugin/v99.0.0')
  f.git('tag', 'packages/cli/v0.3.0-rc.1')
  const bin = join(f.root, 'bin')
  const original = join(f.root, 'published')
  const dist = join(f.checkout, 'dist')
  const log = join(f.root, 'gh-calls.jsonl')
  for (const directory of [bin, original, dist]) mkdirSync(directory)
  const names = [
    `tokeninsights_${version}_linux_amd64.tar.gz`,
    `tokeninsights_${version}_darwin_arm64.tar.gz`,
  ]
  for (const name of names) {
    writeFileSync(join(original, name), `original ${name}`)
    writeFileSync(join(dist, name), `rebuilt ${name}`)
  }
  writeFileSync(
    join(original, 'checksums.txt'),
    names.map((name) => checksum(name, `original ${name}`)).join(''),
  )
  writeFileSync(
    join(dist, 'checksums.txt'),
    names.map((name) => checksum(name, `rebuilt ${name}`)).join(''),
  )
  writeFileSync(log, '')
  writeFileSync(
    join(bin, 'gh'),
    `#!${process.execPath}
const fs = require('node:fs')
const path = require('node:path')
const args = process.argv.slice(2)
fs.appendFileSync(process.env.FIXTURE_GH_LOG, JSON.stringify(args) + '\\n')
if (args[0] !== 'release') process.exit(2)
if (args[1] === 'view') process.exit(process.env.FIXTURE_RELEASE_EXISTS === '1' ? 0 : 1)
if (args[1] === 'download') {
  const directory = args[args.indexOf('--dir') + 1]
  for (const name of fs.readdirSync(process.env.FIXTURE_PUBLISHED)) {
    fs.copyFileSync(path.join(process.env.FIXTURE_PUBLISHED, name), path.join(directory, name))
  }
} else if (args[1] !== 'create' && args[1] !== 'edit') process.exit(2)
`,
    { mode: 0o755 },
  )
  const publish = (tag: string, existing: boolean) =>
    f.execute('Publish GitHub release', {
      PATH: `${bin}:${f.env.PATH ?? ''}`,
      RELEASE_TAG: tag,
      RELEASE_VERSION: tag.replace('packages/cli/v', ''),
      FIXTURE_GH_LOG: log,
      FIXTURE_RELEASE_EXISTS: existing ? '1' : '0',
      FIXTURE_PUBLISHED: original,
    })
  const calls = (): string[][] =>
    readFileSync(log, 'utf8')
      .trim()
      .split('\n')
      .filter(Boolean)
      .map((line) => {
        const value: unknown = JSON.parse(line)
        assert.ok(Array.isArray(value))
        return value.map((argument: unknown) => {
          assert.equal(typeof argument, 'string')
          return String(argument)
        })
      })
  return { ...f, original, dist, names, publish, calls }
}

void test('retrying an older release preserves original assets and current latest', (t) => {
  const f = publisher()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const result = f.publish('packages/cli/v0.1.0', true)
  assert.equal(result.status, 0, result.stderr)
  const edit = f.calls().find((args) => args[1] === 'edit')
  assert.ok(edit)
  assert.ok(edit.includes('--latest=false'))
  assert.ok(f.calls().every((args) => args[1] !== 'create' && args[1] !== 'upload'))
  for (const name of [...f.names, 'checksums.txt'])
    assert.deepEqual(readFileSync(join(f.dist, name)), readFileSync(join(f.original, name)))
})

void test('retrying the newest stable release keeps latest metadata', (t) => {
  const f = publisher('0.2.0')
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const result = f.publish('packages/cli/v0.2.0', true)
  assert.equal(result.status, 0, result.stderr)
  assert.ok(f.calls().some((args) => args[1] === 'edit' && args.includes('--latest=true')))
})

void test('a new stable release publishes verified archives as latest', (t) => {
  const f = publisher('0.2.0')
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const result = f.publish('packages/cli/v0.2.0', false)
  assert.equal(result.status, 0, result.stderr)
  const create = f.calls().find((args) => args[1] === 'create')
  assert.ok(create)
  assert.ok(create.includes('--latest=true'))
  assert.ok(create.includes('--verify-tag'))
  assert.ok(create.includes('dist/checksums.txt'))
  for (const name of f.names) assert.ok(create.includes(`dist/${name}`))
  assert.ok(f.calls().every((args) => args[1] !== 'download' && args[1] !== 'edit'))
})

for (const failure of ['missing archive', 'incorrect checksum', 'missing checksum entry']) {
  void test(`release recovery rejects ${failure} before publishing or replacing archives`, (t) => {
    const f = publisher()
    t.after(() => rmSync(f.root, { recursive: true, force: true }))
    const [first, second] = f.names
    assert.ok(first)
    assert.ok(second)
    if (failure === 'missing archive') rmSync(join(f.original, first))
    else if (failure === 'incorrect checksum')
      writeFileSync(join(f.original, first), 'corrupted archive')
    else writeFileSync(join(f.original, 'checksums.txt'), checksum(second, `original ${second}`))
    const result = f.publish('packages/cli/v0.1.0', true)
    assert.notEqual(result.status, 0)
    assert.ok(f.calls().every((args) => args[1] !== 'edit' && args[1] !== 'create'))
    for (const name of f.names)
      assert.equal(readFileSync(join(f.dist, name), 'utf8'), `rebuilt ${name}`)
  })
}

for (const mode of ['asset commit', 'legacy source tag']) {
  const name =
    mode === 'asset commit'
      ? 'release recovery bundles the original asset commit without new build tooling'
      : 'release recovery rejects historical main tags before setup'
  void test(name, (t) => {
    const f = fixture()
    t.after(() => rmSync(f.root, { recursive: true, force: true }))
    const source = f.git('rev-parse', 'HEAD')
    const tag = 'packages/cli/v0.1.0'
    let release = source
    if (mode === 'asset commit') {
      mkdirSync(join(f.checkout, embeddedAssets), { recursive: true })
      writeFileSync(
        join(f.checkout, embeddedAssets, 'index.html'),
        '<html>historical dashboard</html>',
      )
      release = prepareRelease(f.checkout, source, tag)
    } else f.git('tag', tag)
    writeFileSync(join(f.checkout, 'source.txt'), 'main advanced after publication\n')
    f.git('add', 'source.txt')
    f.git('commit', '-m', 'newer main')
    f.git('push', 'origin', 'main')
    const output = join(f.root, 'outputs')
    writeFileSync(output, '')
    const pinned = f.execute('Pin source', { RETRY_TAG: tag, GITHUB_OUTPUT: output })
    if (mode === 'legacy source tag') {
      assert.notEqual(pinned.status, 0)
      assert.match(pinned.stderr, /retry_tag requires a generated asset commit/)
      assert.equal(readFileSync(output, 'utf8'), '')
      return
    }
    assert.equal(pinned.status, 0, pinned.stderr)
    const entries = readFileSync(output, 'utf8').trim().split('\n')
    assert.ok(entries.includes(`release=${release}`))
    assert.ok(entries.includes(`commit=${source}`))
    assert.equal(f.git('rev-parse', 'HEAD'), source)
    assert.equal(existsSync(join(f.checkout, 'tools/build/src/install-refs.ts')), false)
    const prepared = f.execute('Prepare installable source commit', {
      EXISTING_RELEASE: release,
      RELEASE_TAG: tag,
      GITHUB_SHA: source,
      GITHUB_OUTPUT: output,
    })
    assert.equal(prepared.status, 0, prepared.stderr)
    assert.ok(readFileSync(output, 'utf8').includes(`commit=${release}\n`))
    assert.equal(f.git('bundle', 'list-heads', 'release.bundle'), `${release} refs/tags/${tag}`)
    assert.doesNotThrow(() => f.git('bundle', 'verify', 'release.bundle'))
  })
}
