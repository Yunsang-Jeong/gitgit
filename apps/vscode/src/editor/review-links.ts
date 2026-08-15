import { parseCanonicalWebRemote } from '../shared/web-remotes.js'

export interface ReviewLink {
  kind: 'pull-request' | 'merge-request'
  label: string
  url: string
}

interface WebRepository {
  baseUrl: string
  host: string
  path: string
  provider: 'github' | 'gitlab' | 'unknown'
}

export function reviewLinkForCommit(
  message: string,
  parentCount: number,
  webRemotes: readonly string[] = [],
): ReviewLink | undefined {
  if (!Number.isSafeInteger(parentCount) || parentCount < 0 || parentCount > 64) return undefined
  const repositories = uniqueRepositories(webRemotes)
  if (repositories.length === 0) return undefined
  const candidates: ReviewLink[] = []

  if (parentCount === 2) {
    const trailer = lastNonemptyLine(message)
    const mergeRequest = trailer.match(/^See merge request\s+([^\s!]+)!([1-9]\d{0,9})$/iu)
    const mergePath = mergeRequest?.[1]
    const mergeNumber = mergeRequest?.[2]
    const matching = mergePath
      ? repositories.filter((repository) => sameRepositoryPath(mergePath, repository.path))
      : []
    const repository = matching.length === 1 ? matching[0] : undefined
    if (mergeNumber && repository?.provider === 'gitlab') {
      candidates.push({
        kind: 'merge-request',
        label: `Open MR !${mergeNumber}`,
        url: `${repository.baseUrl}/-/merge_requests/${mergeNumber}`,
      })
    }
  }

  const onlyRepository = repositories.length === 1 ? repositories[0] : undefined
  const mergePullRequest = parentCount === 2
    ? firstLine(message).match(/^Merge pull request #([1-9]\d{0,9}) from \S+$/iu)
    : undefined
  const pullRequest = mergePullRequest
  const pullNumber = pullRequest?.[1]
  if (pullNumber && onlyRepository?.provider === 'github') {
    candidates.push({
      kind: 'pull-request',
      label: `Open PR #${pullNumber}`,
      url: `${onlyRepository.baseUrl}/pull/${pullNumber}`,
    })
  }

  const unique = new Map(candidates.map((candidate) => [candidate.url, candidate]))
  return unique.size === 1 ? unique.values().next().value : undefined
}

function webRepository(webRemote: string): WebRepository | undefined {
  const parsed = parseCanonicalWebRemote(webRemote)
  if (!parsed) return undefined
  const provider = parsed.host === 'github.com'
      ? 'github'
      : parsed.host === 'gitlab.com' ? 'gitlab' : 'unknown'
  return { ...parsed, provider }
}

function uniqueRepositories(webRemotes: readonly string[]): WebRepository[] {
  const repositories = new Map<string, WebRepository>()
  for (const remote of webRemotes) {
    const repository = webRepository(remote)
    if (repository) repositories.set(repository.baseUrl.toLowerCase(), repository)
  }
  return [...repositories.values()]
}

function sameRepositoryPath(value: string, repositoryPath: string): boolean {
  return value.replace(/^\/+|\/+$/gu, '').replace(/\.git$/iu, '').toLowerCase() === repositoryPath.toLowerCase()
}

function firstLine(value: string): string {
  return value.split(/\r?\n/u, 1)[0] ?? ''
}

function lastNonemptyLine(value: string): string {
  const lines = value.split(/\r?\n/u)
  for (let index = lines.length - 1; index >= 0; index--) {
    const line = lines[index]?.trim()
    if (line) return line
  }
  return ''
}
