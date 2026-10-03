import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { workspaceRoot } from './workspace.ts'

const files = execFileSync(
  'git',
  ['ls-files', '-z', '--cached', '--others', '--exclude-standard', '*.go'],
  { cwd: workspaceRoot, encoding: 'utf8' },
)
  .split('\0')
  .filter((file) => file !== '' && existsSync(`${workspaceRoot}/${file}`))
const check = process.argv.includes('--check')
if (files.length > 0) {
  const output = execFileSync('gofmt', [check ? '-l' : '-w', ...files], {
    cwd: workspaceRoot,
    encoding: 'utf8',
  })
  if (check && output !== '') {
    process.stderr.write(output)
    process.exitCode = 1
  }
}
