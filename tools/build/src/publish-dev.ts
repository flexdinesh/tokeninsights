import { execFileSync } from 'node:child_process'

const maxPublicationAttempts = 3

function git(...args: string[]): string {
  return execFileSync('git', args, { encoding: 'utf8' }).trim()
}

function main(): void {
  const commit = process.env.GITHUB_SHA
  if (!commit || git('rev-parse', 'HEAD') !== commit) {
    throw new Error('Checkout must match the verified GITHUB_SHA')
  }

  for (let attempt = 1; attempt <= maxPublicationAttempts; attempt++) {
    const refs = git('ls-remote', '--heads', 'origin', 'refs/heads/main', 'refs/heads/dev')
      .split('\n')
      .map((line) => line.split(/\s+/))
    const mainCommit = refs.find(([, ref]) => ref === 'refs/heads/main')?.[0]
    const devCommit = refs.find(([, ref]) => ref === 'refs/heads/dev')?.[0] ?? ''
    if (!mainCommit) throw new Error('Remote main branch is missing')

    // A slower or rerun CI job must not roll dev back after a newer main push.
    if (mainCommit !== commit) {
      console.log('Skipping superseded main commit')
      return
    }

    // The first publication may replace the obsolete, independently maintained dev.
    // An explicit lease rejects changes made to dev since the remote read above.
    try {
      git(
        'push',
        `--force-with-lease=refs/heads/dev:${devCommit}`,
        'origin',
        `${commit}:refs/heads/dev`,
      )
      console.log(`Published ${commit} to dev`)
      return
    } catch (error) {
      if (attempt === maxPublicationAttempts) throw error
      console.log('Publication failed; rechecking main and dev before retry')
    }
  }
}

main()
