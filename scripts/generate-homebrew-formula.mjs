import { mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname } from "node:path"

const owner = "flexdinesh"
const repo = "tokeninsights"
const formulaName = "tokeninsights"
const homepage = "https://github.com/flexdinesh/tokeninsights"
const desc = "Local token usage tracking for OpenCode, Pi, and Codex"
const license = "MIT"

const targets = [
  { os: "darwin", arch: "amd64", homebrewOS: "macos", homebrewArch: "intel" },
  { os: "darwin", arch: "arm64", homebrewOS: "macos", homebrewArch: "arm" },
  { os: "linux", arch: "amd64", homebrewOS: "linux", homebrewArch: "intel" },
  { os: "linux", arch: "arm64", homebrewOS: "linux", homebrewArch: "arm" },
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

function parseArgs(args) {
  const parsed = new Map()

  for (let index = 0; index < args.length; index += 2) {
    const key = args[index]
    const value = args[index + 1]
    if (!key?.startsWith("--") || value === undefined || value.startsWith("--")) {
      throw new Error("usage: node scripts/generate-homebrew-formula.mjs --version <version> --tag <tag> --checksums <path> --output <path>")
    }
    parsed.set(key.slice(2), value)
  }

  return {
    version: requireOption(parsed, "version"),
    tag: requireOption(parsed, "tag"),
    checksums: requireOption(parsed, "checksums"),
    output: requireOption(parsed, "output"),
  }
}

function requireOption(parsed, name) {
  const value = parsed.get(name)?.trim()
  if (!value) {
    throw new Error(`missing required option --${name}`)
  }
  return value
}

async function generateFormula({ version, tag, checksums }) {
  const checksumText = await readFile(checksums, "utf8")
  const checksumByArtifact = parseChecksums(checksumText)
  const encodedTag = encodeURIComponent(tag)

  const archives = targets.map((target) => {
    const artifact = `${formulaName}_${version}_${target.os}_${target.arch}.tar.gz`
    const sha256 = checksumByArtifact.get(artifact)
    if (!sha256) {
      throw new Error(`missing checksum for ${artifact}`)
    }
    return {
      ...target,
      artifact,
      sha256,
      url: `https://github.com/${owner}/${repo}/releases/download/${encodedTag}/${artifact}`,
    }
  })

  const archiveByKey = new Map(
    archives.map((archive) => [`${archive.homebrewOS}/${archive.homebrewArch}`, archive]),
  )

  return `class Tokeninsights < Formula
  desc "${desc}"
  homepage "${homepage}"
  version "${version}"
  license "${license}"

  on_macos do
    on_intel do
      url "${archiveByKey.get("macos/intel").url}"
      sha256 "${archiveByKey.get("macos/intel").sha256}"
    end

    on_arm do
      url "${archiveByKey.get("macos/arm").url}"
      sha256 "${archiveByKey.get("macos/arm").sha256}"
    end
  end

  on_linux do
    on_intel do
      url "${archiveByKey.get("linux/intel").url}"
      sha256 "${archiveByKey.get("linux/intel").sha256}"
    end

    on_arm do
      url "${archiveByKey.get("linux/arm").url}"
      sha256 "${archiveByKey.get("linux/arm").sha256}"
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

function parseChecksums(text) {
  const checksums = new Map()

  for (const line of text.split("\n")) {
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
