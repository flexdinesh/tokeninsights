import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { workspacePath } from '../src/workspace.ts'

const copies = [
  ['packages/web/public/favicon.svg', 'favicon.svg'],
  ['packages/web/public/favicon.ico', 'favicon.ico'],
  ['docs-website/public/favicon.svg', 'favicon.svg'],
  ['docs-website/public/lockup-light.svg', 'lockup-light.svg'],
  ['docs-website/public/lockup-dark.svg', 'lockup-dark.svg'],
  ['docs-website/public/social-preview.png', 'social-preview.png'],
]

for (const [copy = '', source = ''] of copies) {
  void test(`${copy} matches assets/brand/${source}`, () => {
    assert.ok(
      readFileSync(workspacePath(copy)).equals(readFileSync(workspacePath('assets/brand', source))),
    )
  })
}
