import type { RemoteInfo } from './types'

export interface ReviewLink {
  kind: 'pull-request' | 'merge-request'
  label: string
  url: string
}

export function buildReviewLink(message: string, parents: string[], remotes: RemoteInfo[], upstream = ''): ReviewLink | null {
  if (parents.length < 2) return null

  const explicitPullRequest = message.match(/(https?:\/\/[^\s)]+\/pull\/(\d+))/i)
  if (explicitPullRequest) {
    return { kind: 'pull-request', label: `PR #${explicitPullRequest[2]}`, url: trimURLPunctuation(explicitPullRequest[1]) }
  }
  const explicitMergeRequest = message.match(/(https?:\/\/[^\s)]+\/-\/merge_requests\/(\d+))/i)
  if (explicitMergeRequest) {
    return { kind: 'merge-request', label: `MR !${explicitMergeRequest[2]}`, url: trimURLPunctuation(explicitMergeRequest[1]) }
  }

  const remote = preferredRemote(remotes, upstream)
  const baseURL = remote ? webRepositoryURL(remote.url) : ''
  if (!baseURL) return null

  const pullRequest = message.match(/Merge pull request #(\d+)/i)
  if (pullRequest) {
    return { kind: 'pull-request', label: `PR #${pullRequest[1]}`, url: `${baseURL}/pull/${pullRequest[1]}` }
  }
  const mergeRequest = message.match(/(?:See merge request\s+[^\s!]+|Merge request\s*)!(\d+)/i)
  if (mergeRequest) {
    return { kind: 'merge-request', label: `MR !${mergeRequest[1]}`, url: `${baseURL}/-/merge_requests/${mergeRequest[1]}` }
  }
  return null
}

function preferredRemote(remotes: RemoteInfo[], upstream: string): RemoteInfo | undefined {
  const byNameLength = [...remotes].sort((left, right) => right.name.length - left.name.length)
  const upstreamRemote = byNameLength.find((remote) => upstream === remote.name || upstream.startsWith(`${remote.name}/`))
  return upstreamRemote ?? remotes.find((remote) => remote.name === 'origin') ?? remotes[0]
}

function webRepositoryURL(remoteURL: string): string {
  const value = remoteURL.trim()
  const scpLike = value.match(/^(?:[^@/\s]+@)?([^:/\s]+):(.+)$/)
  if (scpLike && !value.includes('://')) return cleanRepositoryURL(`https://${scpLike[1]}/${scpLike[2]}`)

  try {
    const parsed = new URL(value)
    if (!['http:', 'https:', 'ssh:', 'git:'].includes(parsed.protocol) || !parsed.host) return ''
    const host = parsed.protocol === 'ssh:' || parsed.protocol === 'git:' ? parsed.hostname : parsed.host
    return cleanRepositoryURL(`https://${host}${parsed.pathname}`)
  } catch {
    return ''
  }
}

function cleanRepositoryURL(value: string): string {
  return value.replace(/\.git$/i, '').replace(/\/+$/, '')
}

function trimURLPunctuation(value: string): string {
  return value.replace(/[.,;:]+$/, '')
}
