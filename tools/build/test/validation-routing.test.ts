import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import test from 'node:test'
import { workspaceRoot } from '../src/workspace.ts'

function dryRun(task: string, args: string[] = []): string {
  const result = spawnSync(
    'mise',
    ['run', '--dry-run', '--force', '--no-deps', '--skip-tools', task, '--', ...args],
    { cwd: workspaceRoot, encoding: 'utf8' },
  )
  assert.equal(result.status, 0, result.stderr)
  return result.stdout + result.stderr
}

void test('public checks enter isolated validation without native check commands', () => {
  for (const task of [
    'lint',
    'check-schema',
    'check-api',
    'check:toolchain',
    'check:plugins',
    'typecheck:plugins',
    'test:js',
    'test:go',
    'test:race',
    'test',
    'bench:smoke',
    'test:distribution',
    'test:web-e2e',
    'test:container',
  ]) {
    const output = dryRun(task)
    assert.match(output, /go run \.\/cmd\/validate --group /, task)
    assert.doesNotMatch(output, /\$ (?:pnpm|node|oxlint|oxfmt|go test|go tool)/, task)
    assert.doesNotMatch(output, /\$ mise deps install/, task)
  }
})

void test('validation forwards caller flags', () => {
  assert.match(
    dryRun('check:pr', ['--group', 'browser', '--force']),
    /go run \.\/cmd\/validate --group browser --force/,
  )
})

void test('development builds retain native leaves and dependency preparation', () => {
  const output = dryRun('build')
  assert.match(output, /go build -o bin\/tokeninsights/)
  assert.match(output, /node tools\/build\/src\/build-web.ts/)
  assert.match(output, /mise deps install/)
  assert.doesNotMatch(output, /go run \.\/cmd\/validate/)
})

void test('format verification never installs dependencies or enters Dagger', () => {
  for (const task of ['format:check', 'check:internal:format']) {
    const output = dryRun(task)
    assert.match(output, /format-go.ts --check/)
    assert.match(output, /oxfmt --check/)
    assert.doesNotMatch(output, /mise deps install|cmd\/validate|dagger/)
  }
})

void test('container leaves never invoke public wrappers', () => {
  for (const group of [
    'static',
    'web',
    'tooling',
    'go',
    'benchmarks',
    'distribution',
    'browser',
    'deployment',
  ]) {
    const output = dryRun('check:internal:' + group)
    assert.doesNotMatch(output, /cmd\/validate|mise run (?:check:pr|test:|lint)/)
  }
  assert.match(dryRun('check:internal:web'), /exec vitest run/)
  assert.doesNotMatch(dryRun('check:internal:web'), /tsc/)
})
