import type { RemoteBranchCatalogEntry, RemoteBranchInfo } from './types'

const remoteRefPrefix = 'refs/remotes/'

export function isRemoteBranchScope(scope: string): boolean {
  return scope.startsWith(remoteRefPrefix) && scope.length > remoteRefPrefix.length
}

export function remoteBranchLabel(ref: string): string {
  return isRemoteBranchScope(ref) ? ref.slice(remoteRefPrefix.length) : ref
}

export function visibleRemoteBranches(catalog: RemoteBranchCatalogEntry[]): RemoteBranchInfo[] {
  return catalog.flatMap((entry) => entry.loaded && !entry.error ? entry.branches : [])
}

export function remoteScopeUnavailable(scope: string, catalog: RemoteBranchCatalogEntry[]): boolean {
  const entry = remoteCatalogEntryForScope(scope, catalog)
  return Boolean(entry?.loaded && !entry.error && !entry.branches.some((branch) => branch.ref === scope))
}

export function remoteRelatedScope(scope: string, catalog: RemoteBranchCatalogEntry[]): string {
  const entry = remoteCatalogEntryForScope(scope, catalog)
  if (!entry?.loaded || entry.error || !entry.default_branch) return ''
  const defaultRef = `${remoteRefPrefix}${entry.remote.name}/${entry.default_branch}`
  return defaultRef === scope || !entry.branches.some((branch) => branch.ref === defaultRef) ? '' : defaultRef
}

export function remoteCatalogEntryForScope(scope: string, catalog: RemoteBranchCatalogEntry[]): RemoteBranchCatalogEntry | undefined {
  if (!isRemoteBranchScope(scope)) return undefined
  return [...catalog]
    .sort((left, right) => right.remote.name.length - left.remote.name.length)
    .find((entry) => scope.startsWith(`${remoteRefPrefix}${entry.remote.name}/`))
}
