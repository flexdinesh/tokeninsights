import { randomUUID } from 'node:crypto'
import { spawn, spawnSync } from 'node:child_process'
import { postgresImage as image } from './postgres-image.ts'

// Root verification always exercises PostgreSQL. Each Go fixture creates and
// drops its own random database; a supplied DSN needs CREATEDB privileges.
const smoke = process.argv.includes('--benchmark-smoke')
const name = `tokeninsights-tests-${randomUUID()}`
let owned = false

function docker(args: string[]): string {
  const result = spawnSync('docker', args, { encoding: 'utf8' })
  if (result.error || result.status !== 0) {
    throw new Error(`PostgreSQL test container failed: ${result.error?.message ?? result.stderr}`)
  }
  return result.stdout.trim()
}

function cleanup() {
  if (owned) {
    owned = false
    spawnSync('docker', ['rm', '-f', name], { stdio: 'ignore' })
  }
}

process.on('exit', cleanup)
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    cleanup()
    process.exit(1)
  })
}

try {
  let dsn = process.env.TOKENINSIGHTS_TEST_POSTGRES_DSN
  if (!dsn) {
    docker([
      'run',
      '--detach',
      '--rm',
      '--name',
      name,
      '--publish',
      '127.0.0.1::5432',
      '--env',
      'POSTGRES_PASSWORD=tokeninsights-contract',
      image,
    ])
    owned = true
    const port = docker(['port', name, '5432/tcp']).match(/^127\.0\.0\.1:(\d+)$/)?.[1]
    if (!port) throw new Error('PostgreSQL test port unavailable')
    const deadline = Date.now() + 60_000
    while (true) {
      // The image's temporary initialization server only listens on a socket.
      // Wait for TCP so tests cannot race final server startup.
      const ready = spawnSync(
        'docker',
        ['exec', name, 'pg_isready', '-h', '127.0.0.1', '-U', 'postgres'],
        {
          stdio: 'ignore',
        },
      )
      if (ready.status === 0) break
      if (Date.now() >= deadline) throw new Error('PostgreSQL test startup timed out')
      // Readiness polls must run sequentially.
      // eslint-disable-next-line no-await-in-loop
      await new Promise((resolve) => setTimeout(resolve, 200))
    }
    dsn = `postgres://postgres:tokeninsights-contract@127.0.0.1:${port}/postgres?sslmode=disable`
  }
  const args = smoke
    ? ['-run', '^$', '-bench', '.', '-benchtime=1x', '-count=1', '-p=1']
    : process.argv.slice(2)
  const child = spawn('go', ['test', ...args, './...'], {
    stdio: 'inherit',
    env: {
      ...process.env,
      TOKENINSIGHTS_TEST_POSTGRES_DSN: dsn,
      TOKENINSIGHTS_BENCHMARK_SMOKE: smoke ? '1' : '',
    },
  })
  process.exitCode = await new Promise<number>((resolve, reject) => {
    child.on('error', reject)
    child.on('exit', (code) => resolve(code ?? 1))
  })
} finally {
  cleanup()
}
