import { execFileSync } from 'node:child_process'

const changes = execFileSync('git', ['status', '--porcelain', '--', 'packages/cli/internal/server/static'], { encoding: 'utf8' })
if (changes.trim()) {
  console.error('Embedded web assets differ. Run pnpm run build:web and commit the generated assets.')
  process.exitCode = 1
}
