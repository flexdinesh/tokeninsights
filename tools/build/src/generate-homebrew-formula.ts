import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname } from 'node:path'

const owner = 'flexdinesh'
const repo = 'tokeninsights'
const formulaName = 'tokeninsights'
const homepage = 'https://github.com/flexdinesh/tokeninsights'
const desc = 'Local token usage tracking for OpenCode, Pi, and Codex'
const license = 'MIT'

interface Target {
  os: string
  arch: string
  homebrewOS: string
  homebrewArch: string
}

interface FormulaOptions {
  version: string
  tag: string
  checksums: string
  output: string
}

interface Archive extends Target {
  artifact: string
  sha256: string
  url: string
}

const targets: Target[] = [
  { os: 'darwin', arch: 'amd64', homebrewOS: 'macos', homebrewArch: 'intel' },
  { os: 'darwin', arch: 'arm64', homebrewOS: 'macos', homebrewArch: 'arm' },
  { os: 'linux', arch: 'amd64', homebrewOS: 'linux', homebrewArch: 'intel' },
  { os: 'linux', arch: 'arm64', homebrewOS: 'linux', homebrewArch: 'arm' },
]

const options = parseArgs(process.argv.slice(2))

try {
  const formula = await generateFormula(options)
  await mkdir(dirname(options.output), { recursive: true })
  await writeFile(options.output, formula)
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error))
  process.exitCode = 1
}

function parseArgs(args: string[]): FormulaOptions {
  const normalizedArgs = args[0] === '--' ? args.slice(1) : args
  const parsed = new Map<string, string>()

  for (let index = 0; index < normalizedArgs.length; index += 2) {
    const key = normalizedArgs[index]
    const value = normalizedArgs[index + 1]
    if (!key?.startsWith('--') || value === undefined || value.startsWith('--')) {
      throw new Error(
        'usage: pnpm run generate:homebrew -- --version <version> --tag <tag> --checksums <path> --output <path>',
      )
    }
    parsed.set(key.slice(2), value)
  }

  return {
    version: requireOption(parsed, 'version'),
    tag: requireOption(parsed, 'tag'),
    checksums: requireOption(parsed, 'checksums'),
    output: requireOption(parsed, 'output'),
  }
}

function requireOption(parsed: ReadonlyMap<string, string>, name: string): string {
  const value = parsed.get(name)?.trim()
  if (!value) {
    throw new Error(`missing required option --${name}`)
  }
  return value
}

async function generateFormula({ version, tag, checksums }: FormulaOptions): Promise<string> {
  const checksumText = await readFile(checksums, 'utf8')
  const checksumByArtifact = parseChecksums(checksumText)
  const encodedTag = encodeURIComponent(tag)

  const archives = targets.map((target) => {
    const artifact = `${formulaName}_${version}_${target.os}_${target.arch}.tar.gz`
    const sha256 = checksumByArtifact.get(artifact)
    if (!sha256) {
      throw new Error(`missing checksum for ${artifact}`)
    }
    return Object.assign({}, target, {
      artifact,
      sha256,
      url: `https://github.com/${owner}/${repo}/releases/download/${encodedTag}/${artifact}`,
    })
  })

  const archiveByKey = new Map<string, Archive>(
    archives.map((archive) => [`${archive.homebrewOS}/${archive.homebrewArch}`, archive]),
  )
  const macosIntel = requireArchive(archiveByKey, 'macos/intel')
  const macosArm = requireArchive(archiveByKey, 'macos/arm')
  const linuxIntel = requireArchive(archiveByKey, 'linux/intel')
  const linuxArm = requireArchive(archiveByKey, 'linux/arm')

  return `class Tokeninsights < Formula
  desc "${desc}"
  homepage "${homepage}"
  version "${version}"
  license "${license}"

  on_macos do
    on_intel do
      url "${macosIntel.url}"
      sha256 "${macosIntel.sha256}"
    end

    on_arm do
      url "${macosArm.url}"
      sha256 "${macosArm.sha256}"
    end
  end

  on_linux do
    on_intel do
      url "${linuxIntel.url}"
      sha256 "${linuxIntel.sha256}"
    end

    on_arm do
      url "${linuxArm.url}"
      sha256 "${linuxArm.sha256}"
    end
  end

  def install
    bin.install "tokeninsights"
  end

  test do
    assert_match "tokeninsights #{version}", shell_output("#{bin}/tokeninsights --version")
  end
end
`
}

function requireArchive(archives: ReadonlyMap<string, Archive>, key: string): Archive {
  const archive = archives.get(key)
  if (archive === undefined) {
    throw new Error(`missing archive for ${key}`)
  }
  return archive
}

function parseChecksums(text: string): Map<string, string> {
  const checksums = new Map<string, string>()

  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) {
      continue
    }
    const match = /^([a-f0-9]{64})\s+\*?(.+)$/.exec(trimmed)
    if (!match) {
      throw new Error(`invalid checksum line: ${line}`)
    }
    checksums.set(match[2], match[1])
  }

  return checksums
}
