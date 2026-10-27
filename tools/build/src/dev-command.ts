import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { parseDevOptions, runDevSession, startDevChild } from './dev-lifecycle.ts'
import { workspaceRoot } from './workspace.ts'

export async function runDevCommand(mode: 'server' | 'combined') {
  const controller = new AbortController()
  const interrupt = () => controller.abort()
  process.once('SIGINT', interrupt)
  process.once('SIGTERM', interrupt)
  try {
    const { apiPort, webPort } = parseDevOptions(process.argv.slice(2), mode)
    const root = join(workspaceRoot, '.tokeninsights-dev')
    const startAPI = () =>
      startDevChild(
        join(workspaceRoot, 'packages/cli/bin/tokeninsights'),
        [
          'web',
          '--sync=false',
          '--open=false',
          '--host',
          '127.0.0.1',
          '--port',
          String(apiPort),
          '--collector-db-path',
          join(root, 'collector.sqlite'),
          '--server-db-path',
          join(root, 'server.sqlite'),
          '--app-db-path',
          join(root, 'app.sqlite'),
        ],
        { env: { ...process.env, TOKENINSIGHTS_ACCESS_TOKEN: '', NO_COLOR: '1' } },
        /Dashboard:\s+(http:\/\/127\.0\.0\.1:\d+)/,
      )
    const startWeb = (apiURL: string) => {
      const webRoot = join(workspaceRoot, 'packages/web')
      const require = createRequire(join(webRoot, 'package.json'))
      const vite = join(dirname(require.resolve('vite/package.json')), 'bin/vite.js')
      return startDevChild(
        process.execPath,
        [vite, '--host', '127.0.0.1', '--port', String(webPort), '--strictPort'],
        {
          cwd: webRoot,
          env: { ...process.env, TOKENINSIGHTS_DEV_API_URL: apiURL, NO_COLOR: '1' },
        },
        /Local:\s+(http:\/\/127\.0\.0\.1:\d+\/)/,
      )
    }
    await runDevSession(startAPI, mode === 'combined' ? startWeb : undefined, controller.signal)
  } catch (error) {
    if (!controller.signal.aborted) {
      console.error(error)
      process.exitCode = 1
    }
  } finally {
    process.removeListener('SIGINT', interrupt)
    process.removeListener('SIGTERM', interrupt)
  }
}
