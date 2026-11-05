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
    if (
      url.protocol !== 'tokeninsights-electron:' ||
      url.hostname !== 'launch' ||
      url.username ||
      url.password ||
      url.port ||
      url.hash ||
      url.pathname !== ''
    )
      return undefined
    const ids = url.searchParams.getAll('id')
    if ([...url.searchParams.keys()].length !== 1 || ids.length !== 1) return undefined
    const id = ids[0]
    if (id === undefined || !/^[a-f0-9]{32}$/.test(id)) return undefined
    return url.href
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
