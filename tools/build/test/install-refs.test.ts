import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { pathToFileURL } from 'node:url'

import { assetCommit, embeddedAssets, prepareRelease, publishEdge } from '../src/install-refs.ts'

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), 'tokeninsights-install-refs-'))
  const remote = join(root, 'remote.git')
  const checkout = join(root, 'checkout')
  const git = (...args: string[]) =>
    execFileSync('git', args, {
      cwd: checkout,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    }).trim()
  mkdirSync(checkout)
  git('init', '--bare', '--initial-branch=main', remote)
  git('init', '--initial-branch=main')
  git('config', 'user.name', 'Distribution fixture')
  git('config', 'user.email', 'fixture@example.invalid')
  git('config', 'commit.gpgsign', 'false')
  git('config', 'tag.gpgsign', 'false')
  git('config', 'core.hooksPath', join(root, 'no-hooks'))
  git('remote', 'add', 'origin', remote)
  writeFileSync(join(checkout, '.gitignore'), `${embeddedAssets}/\n`)
  writeFileSync(join(checkout, 'source.txt'), 'initial source\n')
  git('add', '.')
  git('commit', '-m', 'initial source')
  git('push', 'origin', 'main')
  mkdirSync(join(checkout, embeddedAssets, 'assets'), { recursive: true })
  writeFileSync(join(checkout, embeddedAssets, 'index.html'), '<html>dashboard</html>')
  writeFileSync(join(checkout, embeddedAssets, 'assets', 'app.js'), 'production dashboard')
  const advance = (message: string) => {
    writeFileSync(join(checkout, 'source.txt'), `${message}\n`)
    git('add', 'source.txt')
    git('commit', '-m', message)
    git('push', 'origin', 'main')
    return git('rev-parse', 'HEAD')
  }
  return { root, remote, checkout, git, advance }
}

void test('asset commits preserve source history and the real index', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const source = f.git('rev-parse', 'HEAD')
  writeFileSync(join(f.checkout, 'source.txt'), 'unrelated staged edit\n')
  f.git('add', 'source.txt')
  const index = readFileSync(join(f.checkout, '.git', 'index'))
  const commit = assetCommit(f.checkout, source, 'fixture assets')
  assert.deepEqual(readFileSync(join(f.checkout, '.git', 'index')), index)
  assert.equal(f.git('rev-parse', 'HEAD'), source)
  assert.equal(f.git('rev-parse', `${commit}^1`), source)
  assert.equal(f.git('ls-tree', '-r', '--name-only', source, '--', embeddedAssets), '')
  assert.deepEqual(f.git('diff-tree', '--no-commit-id', '--name-only', '-r', commit).split('\n'), [
    `${embeddedAssets}/assets/app.js`,
    `${embeddedAssets}/index.html`,
  ])
  assert.equal(f.git('show', `${commit}:source.txt`), 'initial source')
  assert.equal(readFileSync(join(f.checkout, 'source.txt'), 'utf8'), 'unrelated staged edit\n')
  assert.throws(() => assetCommit(f.checkout, '0'.repeat(40), 'wrong source'), /selected source/)
})

void test('edge works before a release and skips obsolete or repeated publication', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const firstSource = f.git('rev-parse', 'HEAD')
  const firstEdge = publishEdge(f.checkout, firstSource)
  assert.equal(f.git('rev-parse', 'origin/main'), firstSource)
  assert.equal(f.git('ls-remote', '--heads', 'origin', 'edge').split('\t')[0], firstEdge)
  assert.ok(firstEdge)
  assert.equal(f.git('rev-list', '--parents', '-n', '1', firstEdge).split(' ').length, 2)
  assert.equal(publishEdge(f.checkout, firstSource), undefined)
  const secondSource = f.advance('new source')
  const secondEdge = publishEdge(f.checkout, secondSource)
  assert.notEqual(secondEdge, firstEdge)
  assert.equal(publishEdge(f.checkout, firstSource), undefined)
  assert.equal(f.git('ls-remote', '--heads', 'origin', 'edge').split('\t')[0], secondEdge)
  assert.equal(f.git('ls-tree', '-r', '--name-only', 'main', '--', embeddedAssets), '')
})

void test('superseded first edge publication leaves the channel absent', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const old = f.git('rev-parse', 'HEAD')
  const current = f.advance('main advances before first edge')
  assert.equal(publishEdge(f.checkout, old), undefined)
  assert.equal(f.git('ls-remote', '--heads', 'origin', 'edge'), '')
  const edge = publishEdge(f.checkout, current)
  assert.ok(edge)
  assert.equal(f.git('ls-remote', '--heads', 'origin', 'edge').split('\t')[0], edge)
  assert.equal(f.git('rev-parse', `${edge}^1`), current)
})

void test('stable tags are immutable, rerunnable and ancestors of later edge commits', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const source = f.git('rev-parse', 'HEAD')
  const tag = 'packages/cli/v0.1.0'
  const release = prepareRelease(f.checkout, source, tag)
  assert.equal(f.git('rev-parse', `${tag}^1`), source)
  assert.equal(prepareRelease(f.checkout, source, tag), release)
  assert.equal(f.git('ls-remote', '--tags', 'origin'), '')
  f.git('push', 'origin', tag)
  const newer = f.advance('after release')
  assert.throws(() => prepareRelease(f.checkout, newer, tag), /does not belong/)
  assert.throws(() => prepareRelease(f.checkout, newer, 'v0.2.0'), /invalid module release tag/)
  const edge = publishEdge(f.checkout, newer)
  assert.ok(edge)
  assert.equal(f.git('rev-parse', `${edge}^1`), newer)
  assert.equal(f.git('rev-parse', `${edge}^2`), release)
  assert.doesNotThrow(() => f.git('merge-base', '--is-ancestor', tag, edge))
  assert.equal(f.git('rev-parse', 'HEAD'), newer)
})

void test('publication rejects a checkout outside main and development worker assets', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  f.git('checkout', '-b', 'feature')
  writeFileSync(join(f.checkout, 'feature.txt'), 'unmerged source')
  f.git('add', 'feature.txt')
  f.git('commit', '-m', 'feature')
  const source = f.git('rev-parse', 'HEAD')
  assert.throws(() => publishEdge(f.checkout, source), /must belong to main/)
  writeFileSync(join(f.checkout, embeddedAssets, 'mockServiceWorker.js'), 'development worker')
  assert.throws(() => assetCommit(f.checkout, source, 'bad assets'), /mock worker/)
  rmSync(join(f.checkout, embeddedAssets, 'mockServiceWorker.js'))
  rmSync(join(f.checkout, embeddedAssets, 'index.html'))
  assert.throws(() => assetCommit(f.checkout, source, 'missing dashboard'), /assets missing/)
})

void test('release bundles preserve the install commit across fresh job checkouts', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  const source = f.advance('stable release source')
  const tag = 'packages/cli/v0.1.0'
  const release = prepareRelease(f.checkout, source, tag)
  const bundle = join(f.root, 'release.bundle')
  f.git('bundle', 'create', bundle, tag)
  const native = join(f.root, 'native-job')
  execFileSync('git', ['clone', '--depth=1', pathToFileURL(f.remote).href, native], {
    stdio: 'pipe',
  })
  const git = (...args: string[]) =>
    execFileSync('git', args, { cwd: native, encoding: 'utf8', stdio: 'pipe' }).trim()
  assert.equal(git('rev-parse', '--is-shallow-repository'), 'true')
  assert.equal(git('tag', '--list', tag), '')
  git('fetch', bundle, `refs/tags/${tag}:refs/tags/${tag}`)
  git('checkout', '--detach', release)
  assert.equal(git('rev-parse', 'HEAD'), release)
  assert.equal(git('rev-parse', `${tag}^1`), source)
  assert.equal(
    readFileSync(join(native, embeddedAssets, 'index.html'), 'utf8'),
    '<html>dashboard</html>',
  )
  assert.equal(git('ls-tree', '-r', '--name-only', 'main', '--', embeddedAssets), '')
})

void test('edge publication rejects a lease changed before push advertisement', (t) => {
  const f = fixture()
  t.after(() => rmSync(f.root, { recursive: true, force: true }))
  publishEdge(f.checkout, f.git('rev-parse', 'HEAD'))
  const newer = f.advance('newer main source')
  const competitor = assetCommit(f.checkout, newer, 'concurrent edge publication')
  // Make the competing commit durable, then let it win after the publisher
  // observes edge but before Git advertises refs for the final push.
  f.git('push', 'origin', `${competitor}:refs/fixture/competitor`)
  const realGit = execFileSync('/bin/sh', ['-c', 'command -v git'], { encoding: 'utf8' }).trim()
  const wrapper = join(f.root, 'bin')
  mkdirSync(wrapper)
  writeFileSync(
    join(wrapper, 'git'),
    `#!/bin/sh
if test "$1" = push; then
  ${shellQuote(realGit)} --git-dir=${shellQuote(f.remote)} update-ref refs/heads/edge ${competitor} || exit 1
fi
exec ${shellQuote(realGit)} "$@"
`,
    { mode: 0o755 },
  )
  const previousPath = process.env.PATH
  process.env.PATH = `${wrapper}:${previousPath ?? ''}`
  try {
    assert.throws(() => publishEdge(f.checkout, newer), /stale info/)
  } finally {
    if (previousPath === undefined) delete process.env.PATH
    else process.env.PATH = previousPath
  }
  assert.equal(f.git('ls-remote', '--heads', 'origin', 'edge').split('\t')[0], competitor)
})
