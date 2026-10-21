import assert from 'node:assert/strict'
import { mkdir, mkdtemp, rename, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

import { directoryDigest } from '../src/build-web.ts'

void test('web build digest tracks source content and paths but ignores generated entries', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'tokeninsights-web-digest-'))
  t.after(() => rm(root, { force: true, recursive: true }))
  const generated = new Set(['node_modules', 'dist'])
  await mkdir(join(root, 'src', 'nested'), { recursive: true })
  await writeFile(join(root, 'src', 'nested', 'app.ts'), 'export const value = 1\n')
  const initial = await directoryDigest(root, generated)

  await mkdir(join(root, 'node_modules', '.cache'), { recursive: true })
  await mkdir(join(root, 'dist'))
  await writeFile(join(root, 'node_modules', '.cache', 'stamp.txt'), 'stamp')
  await writeFile(join(root, 'dist', 'index.html'), '<html></html>')
  assert.equal(await directoryDigest(root, generated), initial)

  await writeFile(join(root, 'src', 'nested', 'app.ts'), 'export const value = 2\n')
  const changed = await directoryDigest(root, generated)
  assert.notEqual(changed, initial)

  await rename(join(root, 'src', 'nested', 'app.ts'), join(root, 'src', 'nested', 'main.ts'))
  assert.notEqual(await directoryDigest(root, generated), changed)

  await writeFile(join(root, 'src', 'nested', 'main.ts'), 'export const value = 1\n')
  await rename(join(root, 'src', 'nested', 'main.ts'), join(root, 'src', 'nested', 'app.ts'))
  assert.equal(await directoryDigest(root, generated), initial)
})
