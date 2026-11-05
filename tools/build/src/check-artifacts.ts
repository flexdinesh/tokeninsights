import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { workspaceRoot } from './workspace.ts'

export function generatedArtifact(path: string, bytes: Uint8Array): boolean {
  if (
    /^(?:dist\/|packages\/core\/server\/static\/|packages\/(?:cli|backend)\/bin\/|packages\/desktop\/(?:build|dist|out|release|\.mygo)\/)/.test(
      path,
    )
  )
    return true
  if (/\.(?:tar\.gz|tgz|dmg|deb|AppImage)$|\.app\//.test(path)) return true
  const magic = Buffer.from(bytes.subarray(0, 4)).toString('hex')
  return (
    [
      '7f454c46',
      'feedface',
      'feedfacf',
      'cefaedfe',
      'cffaedfe',
      'cafebabe',
      'bebafeca',
      'cafebabf',
      'bfbafeca',
    ].includes(magic) || Buffer.from(bytes.subarray(0, 2)).toString() === 'MZ'
  )
}
export function checkArtifacts(cwd = workspaceRoot): void {
  const paths = execFileSync('git', ['ls-files', '-z', '--cached'], { cwd, encoding: 'utf8' })
    .split('\0')
    .filter(Boolean)
  const invalid = paths.filter(
    (path) =>
      existsSync(`${cwd}/${path}`) && generatedArtifact(path, readFileSync(`${cwd}/${path}`)),
  )
  if (invalid.length > 0)
    throw new Error(`generated output must remain untracked: ${invalid.join(', ')}`)
}
if (import.meta.main) checkArtifacts()
