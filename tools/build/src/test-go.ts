import { spawn } from 'node:child_process'

const smoke = process.argv.includes('--benchmark-smoke')
const concurrency = process.env.TOKENINSIGHTS_CHECK_CONCURRENCY ?? '2'
if (!/^[1-9]\d*$/.test(concurrency) || !Number.isSafeInteger(Number(concurrency))) {
  throw new Error('TOKENINSIGHTS_CHECK_CONCURRENCY must be a positive integer')
}
const args = smoke
  ? ['-run', '^$', '-bench', '.', '-benchtime=1x', '-count=1']
  : process.argv.slice(2)
const child = spawn(
  'go',
  ['test', '-count=1', `-p=${concurrency}`, `-parallel=${concurrency}`, ...args, './...'],
  {
    stdio: 'inherit',
    env: {
      ...process.env,
      GOMAXPROCS: process.env.GOMAXPROCS ?? concurrency,
      TOKENINSIGHTS_BENCHMARK_SMOKE: smoke ? '1' : '',
    },
  },
)
process.exitCode = await new Promise<number>((resolve, reject) => {
  child.on('error', reject)
  child.on('exit', (code) => resolve(code ?? 1))
})
