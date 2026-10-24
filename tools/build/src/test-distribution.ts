import assert from 'node:assert/strict'
import { execFileSync, spawn, spawnSync } from 'node:child_process'
import {
  constants,
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join, relative } from 'node:path'
import { pathToFileURL } from 'node:url'

import { embeddedAssets, workspacePath } from './workspace.ts'

// Real source installs through Git, with disposable publication refs, configuration,
// module cache and databases. The repository and public remote remain untouched.
const fixture = mkdtempSync(join(tmpdir(), 'tokeninsights-distribution-'))
const checkout = join(fixture, 'checkout')
const remote = join(fixture, 'remote.git')
const modulePath = 'github.com/flexdinesh/tokeninsights/packages/cli'
const commandPath = `${modulePath}/cmd/tokeninsights`
// Resolve the selected compiler before isolating its module cache. Otherwise an
// explicit GOTOOLCHAIN attempts to download itself with checksum verification off.
const selectedGoRoot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
const goBinary = join(selectedGoRoot, 'bin', 'go')
const env = { ...process.env }
for (const name of execFileSync('git', ['rev-parse', '--local-env-vars'], { encoding: 'utf8' })
  .trim()
  .split('\n')) {
  delete env[name]
}
for (const name of Object.keys(env)) {
  if (name.startsWith('TOKENINSIGHTS_')) delete env[name]
}
Object.assign(env, {
  GIT_CONFIG_GLOBAL: join(fixture, 'gitconfig'),
  GIT_CONFIG_NOSYSTEM: '1',
  GIT_TERMINAL_PROMPT: '0',
  GOPROXY: 'direct',
  GOSUMDB: 'off',
  GOWORK: 'off',
  GOTOOLCHAIN: 'local',
  GOROOT: selectedGoRoot,
  GOFLAGS: '',
  GOMODCACHE: join(fixture, 'modules'),
  GOBIN: join(fixture, 'bin'),
  CGO_ENABLED: '0',
  XDG_CONFIG_HOME: join(fixture, 'config'),
  XDG_DATA_HOME: join(fixture, 'data'),
  NO_COLOR: '1',
})

function run(command: string, args: string[], cwd = checkout): string {
  return execFileSync(command, args, {
    cwd,
    env,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim()
}

function git(...args: string[]): string {
  return run('git', args)
}

function publish(mode: 'edge' | 'release', source: string, tag?: string): string {
  const args = [join(checkout, 'tools/build/src/install-refs.ts'), mode]
  if (tag !== undefined) args.push(tag)
  return execFileSync(process.execPath, args, {
    cwd: checkout,
    env: { ...env, GITHUB_SHA: source },
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim()
}

function seedDependencyDownloads(): void {
  const goRoot = workspacePath('packages', 'cli')
  const cache = execFileSync('go', ['env', 'GOMODCACHE'], { cwd: goRoot, encoding: 'utf8' }).trim()
  const modules = execFileSync('go', ['list', '-m', '-f', '{{.Path}} {{.Version}}', 'all'], {
    cwd: goRoot,
    encoding: 'utf8',
    env: { ...process.env, GOWORK: 'off' },
  })
  for (const module of modules.trim().split('\n')) {
    const [path, version] = module.trim().split(' ')
    if (path === undefined || version === undefined || path === modulePath) continue
    const escapedPath = path.replace(/[A-Z]/g, (letter) => `!${letter.toLowerCase()}`)
    const escapedVersion = version.replace(/[A-Z]/g, (letter) => `!${letter.toLowerCase()}`)
    const source = join(cache, 'cache/download', escapedPath, '@v')
    const target = join(fixture, 'modules/cache/download', escapedPath, '@v')
    mkdirSync(target, { recursive: true })
    for (const suffix of ['.info', '.mod', '.zip', '.ziphash']) {
      const name = `${escapedVersion}${suffix}`
      if (existsSync(join(source, name))) {
        cpSync(join(source, name), join(target, name), { mode: constants.COPYFILE_FICLONE })
      }
    }
  }
}

async function dashboard(binary: string, channel: string): Promise<void> {
  const state = mkdtempSync(join(fixture, `${channel}-`))
  const config = join(state, 'config.json')
  writeFileSync(config, '{}\n')
  const child = spawn(
    binary,
    [
      '--config-file',
      config,
      'web',
      '--sync=false',
      '--open=false',
      '--host=127.0.0.1',
      '--port=0',
      '--collector-db-path',
      join(state, 'collector.sqlite'),
      '--server-db-path',
      join(state, 'server.sqlite'),
      '--app-db-path',
      join(state, 'app.sqlite'),
    ],
    { cwd: state, env: { ...env, PATH: '' }, stdio: ['ignore', 'pipe', 'pipe'] },
  )
  let output = ''
  let closed = false
  const stopped = new Promise<number | null>((resolve, reject) => {
    child.on('error', reject)
    child.on('close', (code) => {
      closed = true
      resolve(code)
    })
  })
  const ready = new Promise<string>((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      reject(new Error(`installed ${channel} dashboard did not start: ${output}`))
    }, 15_000)
    child.stdout.on('data', (data: Buffer) => {
      output += data.toString()
      const url = /Dashboard:\s+(http:\/\/127\.0\.0\.1:\d+)/.exec(output)?.[1]
      if (url !== undefined) {
        globalThis.clearTimeout(timer)
        resolve(url)
      }
    })
    child.once('close', () => {
      globalThis.clearTimeout(timer)
      reject(new Error(`installed ${channel} dashboard exited before readiness: ${output}`))
    })
    child.once('error', (error) => {
      globalThis.clearTimeout(timer)
      reject(error)
    })
  })
  child.stderr.on('data', (data: Buffer) => {
    output += data.toString()
  })
  try {
    const url = await ready
    const response = await fetch(url, { signal: AbortSignal.timeout(5000) })
    assert.equal(response.status, 200)
    assert.equal(
      await response.text(),
      readFileSync(workspacePath(embeddedAssets, 'index.html'), 'utf8'),
    )
    const asset = /src="(\/assets\/[^"]+\.js)"/.exec(
      readFileSync(workspacePath(embeddedAssets, 'index.html'), 'utf8'),
    )?.[1]
    assert.ok(asset, 'production index must reference its bundled JavaScript')
    const script = await fetch(`${url}${asset}`, { signal: AbortSignal.timeout(5000) })
    assert.equal(script.status, 200)
    assert.equal(
      await script.text(),
      readFileSync(workspacePath(embeddedAssets, asset.slice(1)), 'utf8'),
    )
  } finally {
    if (!closed) child.kill('SIGTERM')
    const timer = globalThis.setTimeout(() => child.kill('SIGKILL'), 5000)
    try {
      assert.equal(await stopped, 0, `installed ${channel} dashboard shutdown: ${output}`)
    } finally {
      globalThis.clearTimeout(timer)
    }
  }
}

async function install(channel: string, version: RegExp): Promise<string> {
  console.log(`Installing ${commandPath}@${channel} through isolated Git`)
  run(
    goBinary,
    ['install', `${commandPath}@${channel}`, `${modulePath}/cmd/tokeninsights-server@${channel}`],
    fixture,
  )
  const binary = join(fixture, 'bin/tokeninsights')
  const output = run(binary, ['--version'], fixture)
  assert.match(output, version)
  console.log(output)
  const serverOutput = execFileSync(join(fixture, 'bin/tokeninsights-server'), ['--version'], {
    cwd: fixture,
    env: { ...env, PATH: '' },
    encoding: 'utf8',
  }).trim()
  assert.equal(serverOutput, output.replace(/^tokeninsights /, 'tokeninsights-server '))
  console.log(serverOutput)
  await dashboard(binary, channel)
  return output
}

try {
  assert.ok(
    existsSync(workspacePath(embeddedAssets, 'index.html')),
    'run mise run build:web before distribution verification',
  )
  mkdirSync(checkout)
  writeFileSync(join(fixture, 'gitconfig'), '')
  git(
    'config',
    '--global',
    `url.${pathToFileURL(remote).href}.insteadOf`,
    'https://github.com/flexdinesh/tokeninsights',
  )
  git('init', '--bare', '--initial-branch=main', remote)
  git('init', '--initial-branch=main')
  git('config', 'user.name', 'Distribution fixture')
  git('config', 'user.email', 'fixture@example.invalid')
  git('config', 'commit.gpgsign', 'false')
  git('config', 'tag.gpgsign', 'false')
  git('config', 'core.hooksPath', join(fixture, 'no-hooks'))
  git('remote', 'add', 'origin', remote)
  cpSync(workspacePath('packages/cli'), join(checkout, 'packages/cli'), {
    recursive: true,
    filter: (path) => {
      const name = relative(workspacePath('packages/cli'), path)
      return (
        name !== 'bin' &&
        !name.startsWith('bin/') &&
        name !== 'node_modules' &&
        !name.startsWith('node_modules/') &&
        name !== 'internal/server/static' &&
        !name.startsWith('internal/server/static/')
      )
    },
  })
  const tools = join(checkout, 'tools/build/src')
  mkdirSync(tools, { recursive: true })
  for (const source of ['install-refs.ts', 'workspace.ts'])
    cpSync(workspacePath('tools/build/src', source), join(tools, source))
  writeFileSync(join(checkout, 'package.json'), '{"type":"module"}\n')
  writeFileSync(join(checkout, '.gitignore'), `${embeddedAssets}/\n`)
  git('add', '.')
  git('commit', '-m', 'source without generated assets')
  git('push', 'origin', 'main')
  cpSync(workspacePath(embeddedAssets), join(checkout, embeddedAssets), { recursive: true })
  seedDependencyDownloads()

  const first = git('rev-parse', 'HEAD')
  publish('edge', first)
  await install('edge', /^tokeninsights v0\.0\.0-\d{14}-[0-9a-f]{12}(?:\s|$)/)

  const tag = 'packages/cli/v0.1.0'
  const release = publish('release', first, tag)
  assert.equal(publish('release', first, tag), release)
  git('push', 'origin', tag)
  writeFileSync(join(checkout, 'after-release.txt'), 'new main source\n')
  git('add', 'after-release.txt')
  git('commit', '-m', 'source after stable release')
  git('push', 'origin', 'main')
  const newer = git('rev-parse', 'HEAD')
  const edge = publish('edge', newer)
  assert.equal(publish('edge', first), 'edge publication skipped')
  assert.equal(git('ls-remote', '--heads', 'origin', 'edge').split('\t')[0], edge)
  await install('edge', /^tokeninsights v0\.1\.1-0\.\d{14}-[0-9a-f]{12}(?:\s|$)/)
  await install('v0.1.0', /^tokeninsights v0\.1\.0(?:\s|$)/)
  await install('latest', /^tokeninsights v0\.1\.0(?:\s|$)/)
  const main = spawnSync(goBinary, ['install', `${commandPath}@main`], {
    cwd: fixture,
    env,
    encoding: 'utf8',
  })
  assert.notEqual(main.status, 0, '@main must fail without generated assets')
  assert.match(
    `${main.stdout}${main.stderr}`,
    /pattern (?:all:)?static[^\n]*no matching files found/,
  )
  assert.equal(git('ls-tree', '-r', '--name-only', 'main', '--', embeddedAssets), '')
  console.log(
    'Distribution verified: first edge, release rerun, edge ordering, stable/latest, dashboard assets, and missing assets on main.',
  )
} finally {
  // Go extracts dependencies read-only, so removal must restore directory permissions.
  execFileSync('chmod', ['-R', 'u+w', fixture])
  rmSync(fixture, { recursive: true, force: true })
}
