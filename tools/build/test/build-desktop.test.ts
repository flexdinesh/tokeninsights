import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { parseTarget } from '../src/build-desktop.ts'
import {
  archiveName,
  archiveRoot,
  packageProduct,
  products,
  verifyProduct,
  writeChecksums,
} from '../src/build-products.ts'

for (const platform of ['linux/amd64', 'darwin/arm64']) {
  for (const product of products) {
    void test(`${product} ${platform} validates its independent executable/backend layout`, async (t) => {
      const root = await mkdtemp(join(tmpdir(), 'tokeninsights-product-layout-'))
      t.after(() => rm(root, { recursive: true, force: true }))
      const target = parseTarget(platform)
      const desktop = product === 'mygo' || product === 'electron'
      const app = product === 'mygo' ? 'TokenInsights MyGo.app' : 'TokenInsights Electron.app'
      const binaryRoot =
        desktop && target.os === 'darwin' ? join(root, app, 'Contents/MacOS') : root
      const backendRoot =
        product === 'electron'
          ? target.os === 'darwin'
            ? join(root, app, 'Contents/Resources')
            : join(root, 'resources')
          : binaryRoot
      await mkdir(binaryRoot, { recursive: true })
      const binary = join(
        binaryRoot,
        product === 'cli' ? 'tokeninsights' : `tokeninsights-${product}`,
      )
      await writeFile(binary, 'executable', { mode: 0o755 })
      if (product !== 'server') {
        await mkdir(backendRoot, { recursive: true })
        await writeFile(join(backendRoot, 'tokeninsights-backend'), 'backend', { mode: 0o755 })
      }
      const plist = join(root, app, 'Contents/Info.plist')
      if (desktop && target.os === 'darwin')
        await writeFile(plist, `<string>com.flexdinesh.tokeninsights.${product}</string>`)
      await writeFile(join(root, 'LICENSE'), 'MIT')
      await writeFile(join(root, 'build-info.json'), '{}')
      await verifyProduct(root, product, target)
      if (desktop && target.os === 'darwin') {
        await writeFile(plist, '<string>com.example.other</string>')
        await assert.rejects(verifyProduct(root, product, target), /incompatible desktop identity/)
      } else {
        await rm(product === 'server' ? binary : join(backendRoot, 'tokeninsights-backend'))
        await assert.rejects(verifyProduct(root, product, target), /ENOENT/)
      }
    })
  }
}

void test('stable product names cover all supported platforms', () => {
  assert.equal(
    archiveName('mygo', parseTarget('darwin/arm64')),
    'tokeninsights-mygo_darwin_arm64.tar.gz',
  )
  assert.equal(
    archiveName('cli', parseTarget('linux/amd64')),
    'tokeninsights-cli_linux_amd64.tar.gz',
  )
  for (const value of ['windows/amd64', 'linux/386', 'linux', 'darwin/arm64/x'])
    assert.throws(() => parseTarget(value))
})
void test('local product archives include backend, deterministic source metadata and license', async (t) => {
  const dir = await mkdtemp(join(tmpdir(), 'tokeninsights-package-'))
  t.after(() => rm(dir, { recursive: true, force: true }))
  const target = parseTarget('linux/amd64')
  const root = join(dir, archiveRoot('cli', target))
  await mkdir(root)
  await writeFile(join(root, 'tokeninsights'), 'binary', { mode: 0o755 })
  await writeFile(join(root, 'tokeninsights-backend'), 'backend', { mode: 0o755 })
  const metadata = { version: '1.2.3', commit: 'a'.repeat(40), date: '2026-11-03T00:00:00+00:00' }
  const archive = await packageProduct(dir, 'cli', target, dir, metadata)
  const entries = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' })
  for (const entry of ['tokeninsights', 'tokeninsights-backend', 'LICENSE', 'build-info.json'])
    assert.ok(entries.includes(`${archiveRoot('cli', target)}/${entry}`))
  const info: unknown = JSON.parse(await readFile(join(root, 'build-info.json'), 'utf8'))
  assert.deepEqual(info, { format: 1, product: 'cli', ...target, ...metadata })
  await writeChecksums(dir)
  assert.match(
    await readFile(join(dir, 'checksums.txt'), 'utf8'),
    /^[0-9a-f]{64}  tokeninsights-cli_linux_amd64\.tar\.gz\n$/,
  )
  await rm(join(root, 'tokeninsights-backend'))
  await assert.rejects(verifyProduct(root, 'cli', target), /tokeninsights-backend/)
})
