// Only HTTP(S) URLs may leave the shell. Schemes cannot invoke local programs.
export function externalURL(value: string): string | undefined {
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password)
      return undefined
    return url.href
  } catch {
    return undefined
  }
}

export function launchURL(value: string): string | undefined {
  try {
    const url = new URL(value)
    if (url.protocol === 'tokeninsights-electron:' && url.hostname === 'open') {
      if (url.username || url.password || url.port || url.hash || !['', '/'].includes(url.pathname))
        return undefined
      const keys = [...url.searchParams.keys()]
      if (keys.length !== 1 || keys[0] !== 'url') return undefined
      return externalURL(url.searchParams.get('url') ?? '')
    }
    return externalURL(value)
  } catch {
    return undefined
  }
}

export function localNavigation(value: string, backendURL: string): boolean {
  try {
    const url = new URL(value)
    return !url.username && !url.password && url.origin === new URL(backendURL).origin
  } catch {
    return false
  }
}

export function localTarget(value: string): boolean {
  const url = externalURL(value)
  if (!url) return false
  const target = new URL(url)
  return (
    target.protocol === 'http:' && ['127.0.0.1', '[::1]', 'localhost'].includes(target.hostname)
  )
}
