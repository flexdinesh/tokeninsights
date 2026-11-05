import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import {
  expectedArchives,
  prepareTag,
  releasePlatforms,
  releaseTag,
  type ReleaseChannel,
  verifyReleaseAssets,
} from '../src/publish-release.ts'
import { writeChecksums } from '../src/build-products.ts'
import { generatedArtifact } from '../src/check-artifacts.ts'
import { workspacePath } from '../src/workspace.ts'

void test('manual versions produce root stable tags and reject injection or legacy tags', () => {
  assert.equal(releaseTag('1.2.3'), 'v1.2.3')
  for (const version of ['v1.2.3', '01.2.3', '1.2', '1.2.3-rc.1', '1.2.3;echo bad'])
    assert.throws(() => releaseTag(version))
})
void test('source tags point directly at source, retries preserve them and another source rejects', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'tokeninsights-source-tag-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  const remote = join(dir, 'remote.git')
  const source = join(dir, 'source')
  mkdirSync(source)
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    GIT_CONFIG_GLOBAL: '/dev/null',
    GIT_CONFIG_NOSYSTEM: '1',
  }
  for (const name of execFileSync('git', ['rev-parse', '--local-env-vars'], { encoding: 'utf8' })
    .trim()
    .split('\n'))
    delete env[name]
  const git = (...args: string[]) =>
    execFileSync('git', args, { cwd: source, env, encoding: 'utf8', stdio: 'pipe' }).trim()
  git('init', '--bare', remote)
  git('init', '--initial-branch=main')
  git('config', 'user.name', 'Fixture')
  git('config', 'user.email', 'fixture@example.invalid')
  git('config', 'commit.gpgsign', 'false')
  git('config', 'tag.gpgsign', 'false')
  git('config', 'core.hooksPath', '/dev/null')
  git('remote', 'add', 'origin', remote)
  writeFileSync(join(source, 'source.go'), 'package source\n')
  git('add', '.')
  git('commit', '-m', 'source')
  const sha = git('rev-parse', 'HEAD')
  assert.equal(prepareTag('1.2.3', source), 'v1.2.3')
  assert.equal(prepareTag('1.2.3', source), 'v1.2.3')
  assert.equal(git('ls-remote', 'origin', 'refs/tags/v1.2.3').split(/\s+/)[0], sha)
  writeFileSync(join(source, 'source.go'), 'package newer\n')
  git('add', '.')
  git('commit', '-m', 'newer source')
  assert.throws(() => prepareTag('1.2.3', source), /another source commit/)
  assert.equal(git('rev-list', '--count', 'HEAD'), '2')
})
void test('publication requires complete unique verified product assets', async (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'tokeninsights-release-assets-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  for (const name of expectedArchives('stable')) writeFileSync(join(dir, name), `archive ${name}`)
  await writeChecksums(dir)
  await verifyReleaseAssets(dir, 'stable')
  await assert.rejects(verifyReleaseAssets(dir, 'main'), /main release requires exactly 9/)
  const first = expectedArchives('stable')[0]
  assert.ok(first)
  const checksums = readFileSync(join(dir, 'checksums.txt'), 'utf8')
  writeFileSync(join(dir, 'checksums.txt'), checksums + checksums.split('\n')[0] + '\n')
  await assert.rejects(verifyReleaseAssets(dir, 'stable'), /checksum count/)
  writeFileSync(join(dir, 'checksums.txt'), checksums)
  writeFileSync(join(dir, first), 'corrupt')
  await assert.rejects(verifyReleaseAssets(dir, 'stable'), /checksum mismatch/)
  rmSync(join(dir, first))
  await assert.rejects(verifyReleaseAssets(dir, 'stable'), /stable release requires exactly 12/)
})
void test('main publication accepts its archives and stable rejects them', async (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'tokeninsights-main-assets-'))
  t.after(() => rmSync(dir, { recursive: true, force: true }))
  for (const name of expectedArchives('main')) writeFileSync(join(dir, name), `archive ${name}`)
  await writeChecksums(dir)
  await verifyReleaseAssets(dir, 'main')
  await assert.rejects(verifyReleaseAssets(dir, 'stable'), /stable release requires exactly 12/)
})
void test('workflow build matrices match each channel release contract', () => {
  const workflows: ReadonlyArray<readonly [string, ReleaseChannel]> = [
    ['ci.yml', 'main'],
    ['release.yml', 'stable'],
  ]
  for (const [file, channel] of workflows) {
    const workflow = readFileSync(workspacePath(`.github/workflows/${file}`), 'utf8')
    const platforms = [...workflow.matchAll(/"os":"(\w+)","arch":"(\w+)"/g)].map(
      (match) => `${match[1]}/${match[2]}`,
    )
    assert.deepEqual(platforms.toSorted(), releasePlatforms(channel).toSorted())
  }
})
void test('artifact guard rejects generated files and compiled binary signatures', () => {
  for (const path of [
    'dist/product.tar.gz',
    'packages/core/server/static/index.html',
    'packages/desktop-electron/out/main.js',
    'packages/cli/bin/tokeninsights',
    'bundle/Thing.app/Contents/Info.plist',
  ])
    assert.ok(generatedArtifact(path, Buffer.from('data')))
  for (const magic of ['7f454c46', 'cffaedfe', '4d5a0000'])
    assert.ok(generatedArtifact('unexpected-output', Buffer.from(magic, 'hex')))
  assert.ok(
    !generatedArtifact('packages/desktop-mygo/resources/icon.png', Buffer.from('89504e47', 'hex')),
  )
})
void test('workflows publish only prebuilt source releases without generated commits', () => {
  const ci = readFileSync(workspacePath('.github/workflows/ci.yml'), 'utf8')
  const release = readFileSync(workspacePath('.github/workflows/release.yml'), 'utf8')
  assert.match(ci, /needs: \[source, products\]/)
  assert.match(release, /version:/)
  for (const workflow of [ci, release])
    assert.doesNotMatch(workflow, /install-refs|homebrew|git commit|packages\/cli\/v|edge/)
})

for (const scenario of [
  'stable',
  'published stable',
  'main',
  'superseded main',
  'upload failure',
]) {
  void test(`release publication handles ${scenario} without exposing partial assets`, (t) => {
    const dir = mkdtempSync(join(tmpdir(), 'tokeninsights-publisher-'))
    t.after(() => rmSync(dir, { recursive: true, force: true }))
    const out = join(dir, 'dist')
    const bin = join(dir, 'bin')
    const stage = join(dir, 'stage')
    for (const path of [out, bin, stage]) mkdirSync(path)
    const commit = 'a'.repeat(40)
    const channel = scenario.includes('main') ? 'main' : 'stable'
    const version = channel === 'main' ? `main.${commit.slice(0, 12)}` : '1.2.3'
    for (const name of expectedArchives(channel)) {
      const root = name.replace(/\.tar\.gz$/, '')
      const [slug, os, arch] = root.split('_')
      const path = join(stage, root)
      mkdirSync(path)
      writeFileSync(
        join(path, 'build-info.json'),
        JSON.stringify({
          format: 1,
          product: slug?.replace('tokeninsights-', ''),
          os,
          arch,
          version,
          commit,
        }),
      )
      execFileSync('tar', ['-czf', join(out, name), '-C', stage, root])
    }
    const log = join(dir, 'calls.jsonl')
    writeFileSync(log, '')
    writeFileSync(
      join(bin, 'git'),
      `#!${process.execPath}
const fs = require('node:fs'); const args = process.argv.slice(2);
fs.appendFileSync(process.env.PUBLISH_LOG, JSON.stringify(['git', ...args])+'\\n');
if (args[0] === 'rev-parse') console.log(process.env.SOURCE_SHA);
if (args[0] === 'ls-remote') console.log((process.env.SUPERSEDED === '1' ? 'b'.repeat(40) : process.env.SOURCE_SHA)+'\\t'+args.at(-1));
`,
      { mode: 0o755 },
    )
    writeFileSync(
      join(bin, 'gh'),
      `#!${process.execPath}
const fs = require('node:fs'); const args = process.argv.slice(2);
fs.appendFileSync(process.env.PUBLISH_LOG, JSON.stringify(['gh', ...args])+'\\n');
if (args[1] === 'view') { if (process.env.EXISTING === '1') console.log('false'); else process.exit(1); }
if (args[1] === 'upload' && process.env.FAIL_UPLOAD === '1') process.exit(23);
`,
      { mode: 0o755 },
    )
    const result = spawnSync(
      process.execPath,
      [
        '--input-type=module',
        '-e',
        `import { writeChecksums } from ${JSON.stringify(workspacePath('tools/build/src/build-products.ts'))}; import { publish } from ${JSON.stringify(workspacePath('tools/build/src/publish-release.ts'))}; await writeChecksums(${JSON.stringify(out)}); await publish(${JSON.stringify(channel)}, ${JSON.stringify(version)}, ${JSON.stringify(out)});`,
      ],
      {
        encoding: 'utf8',
        env: {
          ...process.env,
          PATH: `${bin}:${process.env.PATH ?? ''}`,
          PUBLISH_LOG: log,
          SOURCE_SHA: commit,
          EXISTING: scenario === 'published stable' || scenario === 'main' ? '1' : '0',
          SUPERSEDED: scenario === 'superseded main' ? '1' : '0',
          FAIL_UPLOAD: scenario === 'upload failure' ? '1' : '0',
        },
      },
    )
    const calls = readFileSync(log, 'utf8')
      .trim()
      .split('\n')
      .filter(Boolean)
      .map((line) => {
        const value: unknown = JSON.parse(line)
        assert.ok(Array.isArray(value))
        return value.map((item: unknown) => String(item))
      })
    const create = calls.findIndex((args) => args[0] === 'gh' && args[2] === 'create')
    const upload = calls.findIndex((args) => args[0] === 'gh' && args[2] === 'upload')
    const edit = calls.findIndex((args) => args[0] === 'gh' && args[2] === 'edit')
    if (scenario === 'published stable' || scenario === 'superseded main') {
      assert.equal(create, -1)
      assert.equal(upload, -1)
      assert.equal(edit, -1)
      assert.equal(result.status, scenario === 'published stable' ? 1 : 0, result.stderr)
    } else {
      assert.ok(create >= 0 && upload > create)
      assert.ok(calls[create]?.includes('--draft'))
      assert.equal(
        calls[upload]?.some((arg) => arg.includes('_darwin_amd64')),
        channel === 'stable',
      )
      if (scenario === 'upload failure') {
        assert.notEqual(result.status, 0)
        assert.equal(edit, -1)
      } else {
        assert.equal(result.status, 0, result.stderr)
        assert.ok(edit > upload)
        assert.ok(calls[edit]?.includes(`--latest=${scenario === 'stable'}`))
      }
    }
  })
}
