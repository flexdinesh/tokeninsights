import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'
import { products } from '../src/build-products.ts'
import { expectedArchives } from '../src/publish-release.ts'
import { workspacePath } from '../src/workspace.ts'

void test('remaining products have standalone installers and a complete native release matrix', () => {
  assert.deepEqual(products, ['cli', 'mygo'])
  assert.equal(expectedArchives('stable').length, 8)
  assert.equal(expectedArchives('main').length, 6)
  assert.ok(expectedArchives('stable').some((name) => name.includes('_darwin_amd64')))
  assert.ok(!expectedArchives('main').some((name) => name.includes('_darwin_amd64')))
  for (const product of products) {
    const path = workspacePath(product === 'cli' ? 'install.sh' : `install-${product}.sh`)
    assert.match(readFileSync(path, 'utf8'), new RegExp(`--product=${product}`))
  }
  const tasks = readFileSync(workspacePath('mise.toml'), 'utf8')
  assert.match(tasks, /go build -o bin\/tokeninsights-backend/)
  assert.doesNotMatch(tasks, /tokeninsights-server|build:plugins|typecheck:plugins/)
  const workspaces = readFileSync(workspacePath('pnpm-workspace.yaml'), 'utf8')
  assert.doesNotMatch(workspaces, /plugin-|@opencode|@earendil-works/)
  for (const path of [
    'Dockerfile',
    '.dockerignore',
    'compose.yaml',
    'install-server.sh',
    'deploy/compose.hosted.yaml',
    'deploy/compose.postgres.yaml',
    '.agents/plugins/marketplace.json',
    '.claude-plugin/marketplace.json',
    'packages/plugin-codex',
    'packages/plugin-claude',
    'packages/plugin-runtime',
    'packages/plugin-opencode',
    'packages/plugin-pi',
  ])
    assert.ok(!existsSync(workspacePath(path)), `retired artifact remains: ${path}`)
})

void test('deployment validation uses isolated personal and SSH services without product image builds', () => {
  const source = readFileSync(workspacePath('tools/validation/main.go'), 'utf8')
  assert.match(source, /openssh-server/)
  assert.match(source, /WithServiceBinding\("personal", service\)/)
  assert.match(source, /TOKENINSIGHTS_TEST_PERSONAL_CONTROL_URL/)
  assert.doesNotMatch(source, /DockerBuild|postgres:|POSTGRES_DSN|BOOTSTRAP_PASSWORD/)
  const runner = readFileSync(workspacePath('tools/validation/cmd/validate/main.go'), 'utf8')
  assert.match(runner, /DAGGER_LEAVE_OLD_ENGINE=true/)
  assert.match(runner, /_EXPERIMENTAL_DAGGER_RUNNER_HOST=docker-container:\/\//)
  assert.match(runner, /dev\.tokeninsights\.owner=validation/)
})
