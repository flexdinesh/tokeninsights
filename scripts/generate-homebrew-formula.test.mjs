import assert from "node:assert/strict"
import { execFile } from "node:child_process"
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"
import { promisify } from "node:util"
import test from "node:test"

const execFileAsync = promisify(execFile)

test("generates a tokeninsights Homebrew formula from release checksums", async () => {
  const workspace = await createFormulaWorkspace()

  await writeChecksums(workspace.checksumsPath, completeChecksums())
  await runGenerator(workspace)

  const formula = await readFile(workspace.outputPath, "utf8")

  assert.match(formula, /class Tokeninsights < Formula/)
  assert.match(formula, /version "0\.0\.7"/)
  assert.match(formula, /license "MIT"/)
  assert.match(
    formula,
    /releases\/download\/packages%2Fcli%2Fv0\.0\.7\/tokeninsights_0\.0\.7_darwin_amd64\.tar\.gz/,
  )
  assert.match(
    formula,
    /releases\/download\/packages%2Fcli%2Fv0\.0\.7\/tokeninsights_0\.0\.7_darwin_arm64\.tar\.gz/,
  )
  assert.match(
    formula,
    /releases\/download\/packages%2Fcli%2Fv0\.0\.7\/tokeninsights_0\.0\.7_linux_amd64\.tar\.gz/,
  )
  assert.match(
    formula,
    /releases\/download\/packages%2Fcli%2Fv0\.0\.7\/tokeninsights_0\.0\.7_linux_arm64\.tar\.gz/,
  )
  assert.match(formula, /sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"/)
  assert.match(formula, /sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"/)
  assert.match(formula, /sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"/)
  assert.match(formula, /sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"/)
  assert.match(formula, /bin\.install "tokeninsights"/)
  assert.match(formula, /assert_match "tokeninsights #\{version\}"/)
})

test("fails when a release archive checksum is missing", async () => {
  const workspace = await createFormulaWorkspace()
  const checksums = completeChecksums().filter((line) => !line.includes("linux_arm64"))
  await writeChecksums(workspace.checksumsPath, checksums)

  await assert.rejects(
    runGenerator(workspace),
    /missing checksum for tokeninsights_0\.0\.7_linux_arm64\.tar\.gz/,
  )
})

test("generates a formula with valid Ruby syntax", async (t) => {
  try {
    await execFileAsync("ruby", ["-v"])
  } catch {
    t.skip("ruby is not available")
    return
  }

  const workspace = await createFormulaWorkspace()
  await writeChecksums(workspace.checksumsPath, completeChecksums())
  await runGenerator(workspace)

  const { stdout } = await execFileAsync("ruby", ["-c", workspace.outputPath])

  assert.match(stdout, /Syntax OK/)
})

async function createFormulaWorkspace() {
  const testRoot = join(".scratch", "test-tmp")
  await mkdir(testRoot, { recursive: true })
  const dir = await mkdtemp(join(testRoot, "tokeninsights-formula-"))
  test.after(async () => {
    await rm(dir, { recursive: true, force: true })
  })

  return {
    checksumsPath: join(dir, "checksums.txt"),
    outputPath: join(dir, "tokeninsights.rb"),
  }
}

function completeChecksums() {
  return [
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  tokeninsights_0.0.7_darwin_amd64.tar.gz",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  tokeninsights_0.0.7_darwin_arm64.tar.gz",
    "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  tokeninsights_0.0.7_linux_amd64.tar.gz",
    "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  tokeninsights_0.0.7_linux_arm64.tar.gz",
  ]
}

async function writeChecksums(path, checksums) {
  await writeFile(path, `${checksums.join("\n")}\n`)
}

async function runGenerator({ checksumsPath, outputPath }) {
  await execFileAsync("node", [
    "scripts/generate-homebrew-formula.mjs",
    "--version",
    "0.0.7",
    "--tag",
    "packages/cli/v0.0.7",
    "--checksums",
    checksumsPath,
    "--output",
    outputPath,
  ])
}
