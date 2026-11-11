// Desktop packaging. The root module and source files remain untouched.
import { execFileSync } from 'node:child_process'
import { cp, mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { workspacePath, workspaceRoot } from './workspace.ts'

export type DesktopOS = 'darwin' | 'linux'
export type DesktopArch = 'amd64' | 'arm64'
export interface DesktopTarget {
  os: DesktopOS
  arch: DesktopArch
}
export type DesktopProduct = 'desktop' | 'desktop-native'
export interface DesktopIdentity {
  name: string
  app: string
  binary: string
  identifier: string
}
// Package directory equals the product name; each product owns its MyGo identity.
export const desktopIdentities: Readonly<Record<DesktopProduct, DesktopIdentity>> = {
  desktop: {
    name: 'TokenInsights Desktop',
    app: 'TokenInsights Desktop.app',
    binary: 'tokeninsights-desktop',
    identifier: 'com.flexdinesh.tokeninsights.desktop',
  },
  'desktop-native': {
    name: 'TokenInsights Native',
    app: 'TokenInsights Native.app',
    binary: 'tokeninsights-native',
    identifier: 'com.flexdinesh.tokeninsights.native',
  },
}

export function hostTarget(): DesktopTarget {
  const os = process.platform
  const arch = process.arch === 'x64' ? 'amd64' : process.arch
  return parseTarget(`${os}/${arch}`)
}
export function parseTarget(value: string): DesktopTarget {
  const [os, arch, extra] = value.split('/')
  if (
    (os !== 'darwin' && os !== 'linux') ||
    (arch !== 'amd64' && arch !== 'arm64') ||
    extra !== undefined
  )
    throw new Error(`unsupported desktop platform ${value}`)
  return { os, arch }
}

export async function buildDesktop(
  target: DesktopTarget,
  version: string,
  ldflags: string,
  output: string,
  product: DesktopProduct = 'desktop',
): Promise<void> {
  const { name, app, binary, identifier } = desktopIdentities[product]
  const temporary = await mkdtemp(join(tmpdir(), 'tokeninsights-desktop-'))
  try {
    await Promise.all(
      ['go.mod', 'go.sum'].map((file) => cp(workspacePath(file), join(temporary, file))),
    )
    await Promise.all(
      ['core', product].map((dir) =>
        cp(workspacePath('packages', dir), join(temporary, 'packages', dir), {
          recursive: true,
          filter: (path) => !/[/\\](node_modules|build|\.mygo)$/.test(path),
        }),
      ),
    )
    const project = join(temporary, 'packages', product)
    const configPath = join(project, 'mygo.json')
    const config: unknown = JSON.parse(await readFile(configPath, 'utf8'))
    if (typeof config !== 'object' || config === null || Array.isArray(config))
      throw new Error('invalid mygo.json')
    await writeFile(
      configPath,
      `${JSON.stringify({ ...config, name, slug: binary, identifier, version })}\n`,
    )
    // Resolve the pinned tool from the root module before entering the temporary copy.
    const tool = execFileSync('go', ['tool', '-n', 'mygo'], {
      cwd: workspaceRoot,
      encoding: 'utf8',
    }).trim()
    execFileSync(
      tool,
      ['build', '-platform', `${target.os}/${target.arch}`, '-skip-dmg', '-o', 'build'],
      {
        cwd: project,
        stdio: 'inherit',
        env: { ...process.env, CGO_ENABLED: '0', MYGO_LDFLAGS: ldflags },
      },
    )
    const built = join(project, 'build', `${target.os}-${target.arch}`)
    await mkdir(output, { recursive: true })
    if (target.os === 'darwin') {
      await cp(join(built, app), join(output, app), {
        recursive: true,
        verbatimSymlinks: true,
      })
      await rename(
        join(output, app, 'Contents/MacOS', name),
        join(output, app, 'Contents/MacOS', binary),
      )
      const plist = join(output, app, 'Contents/Info.plist')
      await writeFile(
        plist,
        (await readFile(plist, 'utf8')).replace(
          /(<key>CFBundleExecutable<\/key>\s*<string>)[^<]+/,
          `$1${binary}`,
        ),
      )
    } else {
      await cp(join(built, binary), join(output, binary))
      await cp(join(built, `${binary}.png`), join(output, `${binary}.png`))
    }
  } finally {
    await rm(temporary, { recursive: true, force: true })
  }
}
