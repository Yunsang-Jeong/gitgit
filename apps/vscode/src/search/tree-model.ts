import type {
  SearchFile,
  SearchMatchedFile,
  SearchPatternSource,
  SearchResult,
} from '../helper/protocol.js'
import type { SearchDraft, SearchViewState } from './model.js'
import { isCommitObjectID, isSafeRepositoryRelativePath } from '../shared/repository.js'

export type SearchFileGroup = 'matched' | 'changed'

export interface SearchCommitNode {
  kind: 'commit'
  id: string
  result: SearchResult
  label: string
  description: string
  accessibilityLabel: string
  children: SearchGroupNode[]
}

export interface SearchGroupNode {
  kind: 'group'
  id: string
  commit: SearchResult
  group: SearchFileGroup
  label: string
  description: string
  children: SearchFileNode[]
}

export interface SearchFileNode {
  kind: 'file'
  id: string
  commit: SearchResult
  group: SearchFileGroup
  file: SearchFile | SearchMatchedFile
  label: string
  description: string
  accessibilityLabel: string
}

export type SearchTreeNode = SearchCommitNode | SearchGroupNode | SearchFileNode

export interface SearchFileSelection {
  commit: string
  path: string
  oldPath?: string
  status?: string
}

const SOURCE_LABELS: Record<SearchPatternSource, string> = {
  msg: 'Message',
  diff: 'Diff',
  file: 'Path',
}

export function searchTree(results: readonly SearchResult[], now = new Date()): SearchCommitNode[] {
  return results.map((result) => commitNode(result, now))
}

export function searchFileSelection(value: unknown): SearchFileSelection | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const record = value as Record<string, unknown>
  if (record.kind === 'file') {
    const node = value as SearchFileNode
    return validatedSelection(node.commit.commit, node.file)
  }
  if (typeof record.commit !== 'string' || typeof record.path !== 'string') return undefined
  return validatedSelection(record.commit, {
    status: typeof record.status === 'string' ? record.status : '',
    path: record.path,
    ...(typeof record.oldPath === 'string' ? { oldPath: record.oldPath } : {}),
  })
}

export function searchFilterSummary(draft: SearchDraft): string {
  const values = [draft.allRefs ? 'All refs' : 'HEAD', draft.engine === 'regex' ? 'Regex' : 'Glob']
  if (draft.author.trim()) values.push(`Author: ${draft.author.trim()}`)
  if (draft.since.trim()) values.push(`Since: ${draft.since.trim()}`)
  if (draft.until.trim()) values.push(`Until: ${draft.until.trim()}`)
  if (draft.followRename) values.push('Follow renames')
  return values.join(' · ')
}

export function searchStatusMessage(
  state: SearchViewState,
  runtime: { phase: string, message: string },
): string {
  if (runtime.phase !== 'ready') return runtime.message
  if (state.phase === 'running') {
    return state.progress
      ? `Searching ${state.progress.scanned} of ${state.progress.total} commits…`
      : 'Searching commits…'
  }
  if (state.error) return state.notice ? `${state.error} · ${state.notice}` : state.error
  if (state.phase === 'stale') {
    return `${state.results.length} previous results · Filters changed. Run Search again.`
  }
  if (state.phase === 'ready') {
    const message = state.results.length === 0
      ? `No commits match · ${searchFilterSummary(state.draft)}`
      : `${state.results.length}${state.hasMore ? '+' : ''} commits · ${state.scanned} scanned · ${searchFilterSummary(state.draft)}`
    return state.notice ? `${message} · ${state.notice}` : message
  }
  if (state.notice) return state.notice
  return `Search commit messages, diffs, and paths · ${searchFilterSummary(state.draft)}`
}

export function commitSubject(message: string): string {
  return message.split(/\r?\n/u, 1)[0]?.trim() || '(no commit message)'
}

export function formatRelativeDate(value: string, now = new Date()): string {
  const date = new Date(value)
  const elapsed = now.getTime() - date.getTime()
  if (!Number.isFinite(elapsed)) return 'unknown date'
  if (elapsed < 0) return 'in the future'
  const seconds = Math.floor(elapsed / 1_000)
  if (seconds < 60) return 'now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d`
  const months = Math.floor(days / 30)
  if (days < 365) return `${months}mo`
  return `${Math.floor(days / 365)}y`
}

export function matchSummary(result: SearchResult): string {
  const matched = result.matchedFiles.length
  const changed = result.changedFiles.length
  const message = result.matchSources.includes('msg') ? 'Message' : ''
  if (matched > 0) return `${matched} matched · ${changed} changed`
  if (message) return `${message} match · ${changed} changed`
  return `${changed} changed`
}

function commitNode(result: SearchResult, now: Date): SearchCommitNode {
  const ref = result.refs[0]
  const relativeDate = formatRelativeDate(result.date, now)
  const summary = matchSummary(result)
  const description = [relativeDate, result.author.name, ref, summary].filter(Boolean).join(' · ')
  const groups = fileGroups(result)
  return {
    kind: 'commit',
    id: `commit:${result.commit}`,
    result,
    label: commitSubject(result.message),
    description,
    accessibilityLabel: `${commitSubject(result.message)}, ${result.author.name}, ${relativeDate}, ${summary}`,
    children: groups,
  }
}

function fileGroups(result: SearchResult): SearchGroupNode[] {
  const matched = result.matchedFiles.map((file, index) => fileNode(result, file, 'matched', index))
  const matchedKeys = new Set(result.matchedFiles.map(fileKey))
  const remaining = result.changedFiles
    .filter((file) => !matchedKeys.has(fileKey(file)))
    .map((file, index) => fileNode(result, file, 'changed', index))
  const groups: SearchGroupNode[] = []
  if (matched.length > 0) {
    groups.push({
      kind: 'group',
      id: `group:${result.commit}:matched`,
      commit: result,
      group: 'matched',
      label: 'Matched files',
      description: String(matched.length),
      children: matched,
    })
  }
  if (remaining.length > 0 || (matched.length === 0 && result.changedFiles.length > 0)) {
    const changed = matched.length === 0
      ? result.changedFiles.map((file, index) => fileNode(result, file, 'changed', index))
      : remaining
    groups.push({
      kind: 'group',
      id: `group:${result.commit}:changed`,
      commit: result,
      group: 'changed',
      label: matched.length > 0 ? 'Other changed files' : 'Changed files',
      description: String(changed.length),
      children: changed,
    })
  }
  return groups
}

function fileNode(
  commit: SearchResult,
  file: SearchFile | SearchMatchedFile,
  group: SearchFileGroup,
  index: number,
): SearchFileNode {
  const separator = file.path.lastIndexOf('/')
  const directory = separator >= 0 ? file.path.slice(0, separator) : ''
  const sources = 'matchSources' in file
    ? file.matchSources.map((source) => SOURCE_LABELS[source]).join(' + ')
    : ''
  const details = [file.status, directory, sources].filter(Boolean).join(' · ')
  return {
    kind: 'file',
    id: `file:${commit.commit}:${group}:${index}:${file.path}`,
    commit,
    group,
    file,
    label: separator >= 0 ? file.path.slice(separator + 1) : file.path,
    description: details,
    accessibilityLabel: `${file.path}, status ${file.status}${sources ? `, matched ${sources}` : ''}`,
  }
}

function fileKey(file: SearchFile): string {
  return `${file.status}\u0000${file.oldPath ?? ''}\u0000${file.path}`
}

function validatedSelection(
  commit: string,
  file: { status: string, path: string, oldPath?: string },
): SearchFileSelection | undefined {
  if (!isCommitObjectID(commit) || !isSafeRepositoryRelativePath(file.path)) return undefined
  if (file.oldPath && !isSafeRepositoryRelativePath(file.oldPath)) return undefined
  if (file.status && !/^[A-Z?][A-Z0-9?]{0,7}$/u.test(file.status)) return undefined
  return {
    commit,
    path: file.path,
    ...(file.oldPath ? { oldPath: file.oldPath } : {}),
    ...(file.status ? { status: file.status } : {}),
  }
}
