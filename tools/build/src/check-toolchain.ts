import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspacePath } from './workspace.ts'

// mise.toml owns development tool versions. The check image installs Go, Node
// and pnpm from mise.lock with the mise binary pinned to min_version;
// packageManager metadata and the Go toolchain directive copy the pins.
export type ToolchainFiles = {
  miseToml: string
  dockerfile: string
  packageJson: string
  goMod: string
}

type PinnedTool = 'go' | 'node' | 'pnpm'
const pinnedTools: readonly PinnedTool[] = ['go', 'node', 'pnpm']

function misePins(miseToml: string): Map<PinnedTool, string> {
  const pins = new Map<PinnedTool, string>()
  let inTools = false
  for (const line of miseToml.split('\n')) {
    if (line.startsWith('[')) inTools = line.trim() === '[tools]'
    if (!inTools) continue
    const match = /^(\w+) = "([^"]+)"$/.exec(line.trim())
    const tool = pinnedTools.find((name) => name === match?.[1])
    if (tool !== undefined && match?.[2] !== undefined) pins.set(tool, match[2])
  }
  return pins
}

function minVersion(miseToml: string): string | undefined {
  return /^min_version = "([^"]+)"$/m.exec(miseToml)?.[1]
}

// Separate tool pins in the image would bypass mise.lock and drift silently.
function imageToolPins(dockerfile: string): string[] {
  const pins = /^FROM (?:golang|node):|\bpnpm@\d|\bnpm install --global\b/m
  return pins.test(dockerfile)
    ? ['check image Dockerfile: install Go, Node and pnpm only through mise.lock']
    : []
}

function packageManager(packageJson: string): string | undefined {
  const parsed: unknown = JSON.parse(packageJson)
  if (typeof parsed !== 'object' || parsed === null || !('packageManager' in parsed)) {
    return undefined
  }
  const value = parsed.packageManager
  return typeof value === 'string' ? /^pnpm@([^+]+)/.exec(value)?.[1] : undefined
}

function compare(
  label: string,
  expected: string | undefined,
  actual: string | undefined,
): string | undefined {
  if (actual === undefined) return `${label}: version not found; expected ${expected}`
  return actual === expected ? undefined : `${label}: ${actual} differs from mise.toml ${expected}`
}

export function toolchainMismatches(files: ToolchainFiles): string[] {
  const pins = misePins(files.miseToml)
  const missing = pinnedTools
    .filter((tool) => pins.get(tool) === undefined)
    .map((tool) => `mise.toml: [tools] ${tool} pin missing`)
  if (missing.length > 0) return missing
  const go = pins.get('go')
  const pnpm = pins.get('pnpm')
  const mise = minVersion(files.miseToml)
  if (mise === undefined) return ['mise.toml: min_version missing']
  return [
    compare('check image mise', mise, /^ENV MISE_VERSION=(\S+)$/m.exec(files.dockerfile)?.[1]),
    ...imageToolPins(files.dockerfile),
    compare('package.json packageManager', pnpm, packageManager(files.packageJson)),
    compare(
      'go.mod toolchain',
      go,
      /^toolchain go(\S+)$/m.exec(files.goMod)?.[1] ?? /^go (\S+)$/m.exec(files.goMod)?.[1],
    ),
  ].filter((mismatch) => mismatch !== undefined)
}

export async function readToolchainFiles(): Promise<ToolchainFiles> {
  const [miseToml, dockerfile, packageJson, goMod] = await Promise.all([
    readFile(workspacePath('mise.toml'), 'utf8'),
    readFile(workspacePath('tools/validation/Dockerfile'), 'utf8'),
    readFile(workspacePath('package.json'), 'utf8'),
    readFile(workspacePath('go.mod'), 'utf8'),
  ])
  return { miseToml, dockerfile, packageJson, goMod }
}

async function main(): Promise<void> {
  const mismatches = toolchainMismatches(await readToolchainFiles())
  if (mismatches.length > 0) {
    for (const mismatch of mismatches) console.error(mismatch)
    console.error('Update the listed files to the mise.toml versions.')
    process.exit(1)
  }
  console.log('toolchain pins OK')
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main()
}
