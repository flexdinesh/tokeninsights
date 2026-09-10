import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

const tempRoot = process.env.TOKENINSIGHTS_TEST_TMP ?? tmpdir()
const home = await mkdtemp(join(tempRoot, 'tokeninsights-web-'))
const sessions = join(home, '.pi/agent/sessions')
await mkdir(sessions, { recursive: true })
const now = new Date()
for (let index = 0; index < 80; index++) {
  const session = `web-session-${String(index).padStart(3, '0')}`
  const model = index % 2 === 0 ? 'model-a' : 'model-b'
  const provider = index % 2 === 0 ? 'openai' : 'anthropic'
  const recordedAt = new Date(now)
  if (index >= 60) recordedAt.setFullYear(now.getFullYear() - 1)
  const records = [
    { type: 'session', version: 1, id: session, timestamp: recordedAt.toISOString() },
    { type: 'message', id: 'turn-1', timestamp: recordedAt.toISOString(), message: { role: 'assistant', model, provider, timestamp: recordedAt.getTime(), usage: { input: 1000, output: 200, cacheRead: 3000, cacheWrite: 100, totalTokens: 4300 } } },
  ]
  await writeFile(join(sessions, `${session}.jsonl`), records.map(record => JSON.stringify(record)).join('\n'))
}
const child = spawn(resolve('../cli/bin/tokeninsights'), ['serve', '--week', '--host', '127.0.0.1', '--port', '18765', '--db-path', join(home, 'usage.sqlite')], {
  stdio: 'inherit',
  env: { ...process.env, HOME: home, XDG_DATA_HOME: join(home, '.local/share'), CODEX_HOME: join(home, '.codex'), CLAUDE_CONFIG_DIR: join(home, '.claude') },
})
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal))
child.on('exit', async code => { await rm(home, { recursive: true, force: true }); process.exitCode = code ?? 0 })
child.on('error', async error => { console.error(error); await rm(home, { recursive: true, force: true }); process.exitCode = 1 })
