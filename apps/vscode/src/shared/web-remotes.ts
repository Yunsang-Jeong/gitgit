export interface CanonicalWebRemote {
  baseUrl: string
  host: string
  path: string
}

export function parseCanonicalWebRemote(value: unknown): CanonicalWebRemote | undefined {
  if (typeof value !== 'string' || /[()[\]<>"]|'/u.test(value)) return undefined
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'https:'
      || parsed.username
      || parsed.password
      || parsed.search
      || parsed.hash
      || !safeHostname(parsed.hostname)
      || parsed.pathname === '/'
      || parsed.pathname.endsWith('/')
      || parsed.pathname.toLowerCase().endsWith('.git')
      || parsed.toString() !== value) return undefined
    const path = decodeURIComponent(parsed.pathname).replace(/^\/+|\/+$/gu, '')
    if (!path || path.split('/').some((part) => !part || part === '.' || part === '..')) return undefined
    return { baseUrl: value, host: parsed.host, path }
  } catch {
    return undefined
  }
}

function safeHostname(hostname: string): boolean {
  return hostname.split('.').every((label) => (
    /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/u.test(label)
  ))
}
