import { randomUUID } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { postgresImage } from './postgres-image.ts'

// Explicit integration target: build and exercise the real image, never a host
// binary mounted into a container. Fixtures own isolated networks and volumes.
const image = `tokeninsights-server-test:${randomUUID()}`
try {
  const build = spawnSync('docker', ['build', '-t', image, '.'], { stdio: 'inherit' })
  if (build.error || build.status !== 0) throw new Error('Server image build failed')
  const tests = spawnSync(
    'go',
    [
      'test',
      './internal/deployment',
      '-run',
      '^TestContainerDeployment$',
      '-count=1',
      '-timeout=5m',
    ],
    {
      cwd: 'packages/cli',
      stdio: 'inherit',
      env: {
        ...process.env,
        TOKENINSIGHTS_TEST_SERVER_IMAGE: image,
        TOKENINSIGHTS_TEST_POSTGRES_IMAGE: postgresImage,
      },
    },
  )
  if (tests.error) throw tests.error
  process.exitCode = tests.status ?? 1
} finally {
  spawnSync('docker', ['image', 'rm', image], { stdio: 'ignore' })
}
