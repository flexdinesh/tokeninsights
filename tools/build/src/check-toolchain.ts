import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { workspacePath } from './workspace.ts'

// mise.toml owns development tool versions. Docker, Corepack-style packageManager
// metadata and the Go toolchain directive copy them and must stay equal.
export type ToolchainFiles = {
  miseToml: string
  dockerfile: string
  packageJson: string
  goMod: string
}

const pinnedTools = ['go', 'node', 'pnpm'] as const
type PinnedTool = (typeof pinnedTools)[number]

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
  const node = pins.get('node')
  const pnpm = pins.get('pnpm')
  return [
    compare('Dockerfile golang image', go, /^FROM golang:([^-@\s]+)/m.exec(files.dockerfile)?.[1]),
    compare('Dockerfile node image', node, /^FROM node:([^-@\s]+)/m.exec(files.dockerfile)?.[1]),
    compare('Dockerfile pnpm install', pnpm, /\bpnpm@([^\s]+)/.exec(files.dockerfile)?.[1]),
    compare('package.json packageManager', pnpm, packageManager(files.packageJson)),
    compare('packages/cli/go.mod toolchain', go, /^toolchain go(\S+)$/m.exec(files.goMod)?.[1]),
  ].filter((mismatch) => mismatch !== undefined)
}

export async function readToolchainFiles(): Promise<ToolchainFiles> {
  const [miseToml, dockerfile, packageJson, goMod] = await Promise.all([
    readFile(workspacePath('mise.toml'), 'utf8'),
    readFile(workspacePath('Dockerfile'), 'utf8'),
    readFile(workspacePath('package.json'), 'utf8'),
    readFile(workspacePath('packages', 'cli', 'go.mod'), 'utf8'),
  ])
  return { miseToml, dockerfile, packageJson, goMod }
}

async function main(): Promise<void> {
  const mismatches = toolchainMismatches(await readToolchainFiles())
  if (mismatches.length > 0) {
    for (const mismatch of mismatches) console.error(mismatch)
    console.error('Update the listed files to the mise.toml [tools] versions.')
    process.exit(1)
  }
  console.log('toolchain pins OK')
}

if (process.argv[1] !== undefined && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main()
}
