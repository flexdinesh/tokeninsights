// Builds the desktop viewer with MyGo and packages one unversioned archive per
// OS/arch: dist/tokeninsights-desktop_<os>_<arch>.tar.gz. install.sh downloads
// these names from the latest release, so names must stay stable.
import { execFileSync } from 'node:child_process'
import { chmod, cp, mkdir, mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { workspacePath } from './workspace.ts'

export type DesktopOS = 'darwin' | 'linux'
export type DesktopArch = 'amd64' | 'arm64'

export interface DesktopTarget {
  os: DesktopOS
  arch: DesktopArch
}

const slug = 'tokeninsights-desktop'
const appBundle = 'TokenInsights.app'
const bundleIdentifier = 'com.flexdinesh.tokeninsights'
const urlScheme = 'tokeninsights'
const linuxIcon = `${slug}-512x512.png`
const executableBits = 0o111
const releaseVersion = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

export function archiveName(target: DesktopTarget): string {
  return `${slug}_${target.os}_${target.arch}.tar.gz`
}

export function packageRoot(target: DesktopTarget): string {
  return `${slug}_${target.os}_${target.arch}`
}

export function hostTarget(): DesktopTarget {
  const os = process.platform === 'darwin' ? 'darwin' : process.platform === 'linux' ? 'linux' : ''
  const arch = process.arch === 'x64' ? 'amd64' : process.arch === 'arm64' ? 'arm64' : ''
  if (os === '' || arch === '')
    throw new Error(`unsupported desktop host ${process.platform}/${process.arch}`)
  return { os, arch }
}

export function parseTarget(value: string): DesktopTarget {
  const [os, arch, extra] = value.split('/')
  if ((os !== 'darwin' && os !== 'linux') || (arch !== 'amd64' && arch !== 'arm64') || extra)
    throw new Error(`unsupported desktop platform ${value}; use darwin|linux/amd64|arm64`)
  return { os, arch }
}

// Copies the files a release needs from MyGo's build directory into a package
// directory, then verifies the layout install.sh relies on.
export async function stagePackage(
  buildDir: string,
  target: DesktopTarget,
  stageDir: string,
): Promise<string> {
  const root = join(stageDir, packageRoot(target))
  await rm(root, { recursive: true, force: true })
  await mkdir(root, { recursive: true })
  if (target.os === 'darwin') {
    await cp(join(buildDir, appBundle), join(root, appBundle), {
      recursive: true,
      verbatimSymlinks: true,
    })
  } else {
    await cp(join(buildDir, slug), join(root, slug))
    await chmod(join(root, slug), 0o755)
    await cp(join(buildDir, linuxIcon), join(root, `${slug}.png`))
  }
  await cp(workspacePath('LICENSE'), join(root, 'LICENSE'))
  await verifyPackage(root, target)
  return root
}

export async function verifyPackage(root: string, target: DesktopTarget): Promise<void> {
  if (target.os === 'darwin') {
    const plist = await readFile(join(root, appBundle, 'Contents', 'Info.plist'), 'utf8')
    for (const value of [bundleIdentifier, urlScheme]) {
      if (!plist.includes(`<string>${value}</string>`))
        throw new Error(`${appBundle} Info.plist lacks ${value}`)
    }
    await requireExecutable(join(root, appBundle, 'Contents', 'MacOS', 'TokenInsights'))
  } else {
    await requireExecutable(join(root, slug))
    await stat(join(root, `${slug}.png`))
  }
  await stat(join(root, 'LICENSE'))
}

async function requireExecutable(path: string): Promise<void> {
  const info = await stat(path)
  if (!info.isFile() || (info.mode & executableBits) === 0)
    throw new Error(`${path} is not an executable file`)
}

// Writes the archive; COPYFILE_DISABLE keeps macOS tar from adding AppleDouble files.
export function writeArchive(stageDir: string, target: DesktopTarget, outDir: string): string {
  const archive = join(outDir, archiveName(target))
  execFileSync('tar', ['-C', stageDir, '-czf', archive, packageRoot(target)], {
    env: { ...process.env, COPYFILE_DISABLE: '1' },
    stdio: 'inherit',
  })
  return archive
}

// MyGo reads the version from mygo.json; build a copy so the tracked file never changes.
async function buildApp(target: DesktopTarget, version: string | undefined): Promise<string> {
  const source = workspacePath('packages', 'desktop')
  const work = await mkdtemp(join(tmpdir(), 'tokeninsights-desktop-'))
  const project = join(work, 'desktop')
  await cp(source, project, {
    recursive: true,
    filter: (path) => !/[/\\](build|\.mygo)$/.test(path),
  })
  if (version !== undefined) {
    const configPath = join(project, 'mygo.json')
    const config: unknown = JSON.parse(await readFile(configPath, 'utf8'))
    if (typeof config !== 'object' || config === null || Array.isArray(config))
      throw new Error('packages/desktop/mygo.json must be an object')
    await writeFile(configPath, `${JSON.stringify({ ...config, version }, null, 2)}\n`)
  }
  execFileSync(
    'go',
    [
      'tool',
      'mygo',
      'build',
      '-platform',
      `${target.os}/${target.arch}`,
      '-skip-dmg',
      '-o',
      'build',
    ],
    { cwd: project, env: { ...process.env, CGO_ENABLED: '0' }, stdio: 'inherit' },
  )
  return join(project, 'build', `${target.os}-${target.arch}`)
}

function parseArgs(args: string[]): { out: string; version?: string; target: DesktopTarget } {
  const values = new Map<string, string>()
  const list = args[0] === '--' ? args.slice(1) : args
  for (let index = 0; index < list.length; index += 2) {
    const key = list[index]
    const value = list[index + 1]
    if (
      !['--out', '--version', '--platform'].includes(key ?? '') ||
      value === undefined ||
      value.startsWith('--')
    )
      throw new Error(
        'usage: build-desktop.ts --out <dir> [--version <x.y.z>] [--platform <os>/<arch>]',
      )
    values.set(key ?? '', value)
  }
  const out = values.get('--out')
  if (out === undefined) throw new Error('missing --out')
  const version = values.get('--version')
  if (version !== undefined && !releaseVersion.test(version))
    throw new Error(`invalid --version ${version}`)
  const platform = values.get('--platform')
  return { out, version, target: platform === undefined ? hostTarget() : parseTarget(platform) }
}

if (import.meta.main) {
  const options = parseArgs(process.argv.slice(2))
  const outDir = resolve(workspacePath(), options.out)
  const buildDir = await buildApp(options.target, options.version)
  const stageDir = await mkdtemp(join(tmpdir(), 'tokeninsights-desktop-package-'))
  try {
    await stagePackage(buildDir, options.target, stageDir)
    await mkdir(outDir, { recursive: true })
    console.log(writeArchive(stageDir, options.target, outDir))
  } finally {
    await rm(stageDir, { recursive: true, force: true })
    await rm(resolve(buildDir, '..', '..', '..'), { recursive: true, force: true })
  }
}
