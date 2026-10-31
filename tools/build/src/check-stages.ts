import { type ChildProcess, spawn } from 'node:child_process'
import { createWriteStream } from 'node:fs'
import { mkdir, readFile, rm } from 'node:fs/promises'
import { constants } from 'node:os'
import { join, relative, resolve } from 'node:path'
import { createInterface } from 'node:readline'
import { finished } from 'node:stream/promises'

// Runs check:push stages inside the repository gate. Each argument is one stage;
// comma-separated tasks within a stage run in parallel. Phase output goes to
// per-phase logs so the terminal, and agents reading it, see one line per phase.
const failureTailLines = 100
const verbose = process.env.TOKENINSIGHTS_CHECK_VERBOSE === '1'
const concurrency = process.env.TOKENINSIGHTS_CHECK_CONCURRENCY ?? '2'
if (!/^[1-9]\d*$/.test(concurrency) || !Number.isSafeInteger(Number(concurrency))) {
  throw new Error('TOKENINSIGHTS_CHECK_CONCURRENCY must be a positive integer')
}
const stages = process.argv.slice(2).map((stage) => stage.split(','))
if (stages.length === 0 || stages.some((stage) => stage.some((task) => task === ''))) {
  throw new Error('usage: check-stages.ts <task>[,<task>...] ...')
}
const logDirectory = resolve('.check-logs')

interface Phase {
  task: string
  log: string
}

interface Outcome {
  phase: Phase
  code: number
  seconds: number
}

const running = new Set<ChildProcess>()
let stopSignal: NodeJS.Signals | undefined

function signalCode(signal: NodeJS.Signals): number {
  return 128 + constants.signals[signal]
}

const forwardedSignals: NodeJS.Signals[] = ['SIGINT', 'SIGTERM']
for (const signal of forwardedSignals) {
  process.on(signal, () => {
    stopSignal ??= signal
    for (const child of running) child.kill(signal)
  })
}

async function runPhase(phase: Phase): Promise<Outcome> {
  const start = performance.now()
  const log = createWriteStream(phase.log)
  const child = spawn('mise', ['run', '--no-deps', '--jobs', '1', phase.task], {
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  running.add(child)
  const write = (line: string) => {
    log.write(`${line}\n`)
    if (verbose) process.stdout.write(`[${phase.task}] ${line}\n`)
  }
  for (const stream of [child.stdout, child.stderr]) {
    createInterface({ input: stream, crlfDelay: Infinity }).on('line', write)
  }
  const code = await new Promise<number>((resolveCode) => {
    child.on('error', (error) => {
      write(error.message)
      resolveCode(1)
    })
    child.on('close', (exitCode, signal) => {
      resolveCode(exitCode ?? (signal ? signalCode(signal) : 1))
    })
  })
  running.delete(child)
  log.end()
  await finished(log)
  return { phase, code, seconds: Math.round((performance.now() - start) / 1000) }
}

async function report({ phase, code, seconds }: Outcome): Promise<void> {
  if (code === 0) {
    console.log(`ok ${phase.task} ${seconds}s`)
    return
  }
  const log = relative(process.cwd(), phase.log)
  console.log(`FAIL ${phase.task} ${seconds}s (exit ${code})`)
  if (!verbose) {
    const lines = (await readFile(phase.log, 'utf8')).trimEnd().split('\n')
    const tail = lines.slice(-failureTailLines)
    console.log(`--- last ${tail.length} lines of ${log} ---`)
    console.log(tail.join('\n'))
    console.log('---')
  }
  console.log(`log: ${log}`)
  console.log(`rerun: mise run ${phase.task}`)
}

// Fails fast: the first failure stops running siblings and starts nothing new.
// Returns the first failure's exit code, or 0.
async function runStage(phases: Phase[]): Promise<number> {
  const pending = [...phases]
  let failure = 0
  const worker = async () => {
    for (let phase = pending.shift(); phase; phase = pending.shift()) {
      if (failure !== 0 || stopSignal) return
      // Each worker runs its phases sequentially.
      // eslint-disable-next-line no-await-in-loop
      const outcome = await runPhase(phase)
      if (outcome.code !== 0 && (failure !== 0 || stopSignal)) {
        console.log(`stopped ${phase.task}`)
      } else {
        if (outcome.code !== 0) {
          failure = outcome.code
          for (const child of running) child.kill('SIGTERM')
        }
        // eslint-disable-next-line no-await-in-loop
        await report(outcome)
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(Number(concurrency), phases.length) }, worker))
  return failure
}

await rm(logDirectory, { recursive: true, force: true })
await mkdir(logDirectory, { recursive: true })
let index = 0
const plan = stages.map((stage) =>
  stage.map((task) => {
    index += 1
    const name = `${String(index).padStart(2, '0')}-${task.replaceAll(':', '-')}.log`
    return { task, log: join(logDirectory, name) }
  }),
)
for (const phases of plan) {
  // Stages run in order; a later stage starts only after this one succeeds.
  // eslint-disable-next-line no-await-in-loop
  const failure = await runStage(phases)
  if (stopSignal) {
    process.exitCode = signalCode(stopSignal)
    break
  }
  if (failure !== 0) {
    process.exitCode = failure
    break
  }
}
