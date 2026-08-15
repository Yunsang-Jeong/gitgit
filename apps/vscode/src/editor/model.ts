export const MAX_BLAME_WINDOW_LINES = 2_000

export interface OneBasedLineRange {
  startLine: number
  endLine: number
}

export interface ZeroBasedVisibleRange {
  startLine: number
  endLine: number
}

export interface TargetRange {
  startLine: number
  endLine: number
}

export interface DeletionAnchor {
  anchorLine: number
}

export interface HistorySelectionCommit {
  commit: string
  files: Array<{ status?: string, path: string, oldPath?: string }>
}

export interface HistorySelection {
  commit: string
  path: string
  oldPath?: string
  status?: string
}

export type BlameMode = 'auto' | 'off' | 'activeLine' | 'visibleLines'

export function resolveBlameMode(configuredMode: BlameMode, nativeBlameEnabled: boolean): Exclude<BlameMode, 'auto'> {
  if (configuredMode !== 'auto') return configuredMode
  return nativeBlameEnabled ? 'off' : 'activeLine'
}

export function blameAnnotation(commit: string, author: string, authoredAt: string, now = new Date()): string {
  if (isWorkingTreeCommit(commit)) return '  │ GitGit · Working tree'
  const relativeTime = compactRelativeTime(authoredAt, now)
  return `  │ GitGit · ${author}${relativeTime ? ` · ${relativeTime}` : ''}`
}

export function isWorkingTreeCommit(commit: string): boolean {
  return /^(?:0{40}|0{64})$/u.test(commit)
}

export function compactRelativeTime(value: string, now = new Date()): string {
  const relative = relativeTimeParts(value, now)
  if (!relative) return ''
  if (relative.unit === 'now') return 'now'
  const label = `${relative.value}${relative.unit}`
  return relative.future ? `in ${label}` : label
}

export function relativeTimeDescription(value: string, now = new Date()): string {
  const relative = relativeTimeParts(value, now)
  if (!relative) return ''
  if (relative.unit === 'now') return 'just now'
  const names = { m: 'minute', h: 'hour', d: 'day', mo: 'month', y: 'year' } as const
  const unit = names[relative.unit]
  const quantity = `${relative.value} ${unit}${relative.value === 1 ? '' : 's'}`
  return relative.future ? `${quantity} from now` : `${quantity} ago`
}

export function fileHistoryDescription(
  shortCommit: string,
  author: string,
  authoredAt: string,
  now = new Date(),
): string {
  const relativeTime = compactRelativeTime(authoredAt, now)
  return [author, relativeTime, shortCommit].filter(Boolean).join(' · ')
}

export function fileHistoryAccessibilityLabel(
  subject: string,
  shortCommit: string,
  author: string,
  authoredAt: string,
  now = new Date(),
): string {
  const relativeTime = relativeTimeDescription(authoredAt, now)
  return [
    `Commit ${subject || shortCommit}`,
    `Authored by ${author}${relativeTime ? ` ${relativeTime}` : ''}`,
    `Commit ID ${shortCommit}`,
    'Open read-only revision',
  ].join('. ')
}

type RelativeTimeUnit = 'now' | 'm' | 'h' | 'd' | 'mo' | 'y'

interface RelativeTimeParts {
  value: number
  unit: RelativeTimeUnit
  future: boolean
}

function relativeTimeParts(value: string, now: Date): RelativeTimeParts | undefined {
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || Number.isNaN(now.getTime())) return undefined

  const seconds = Math.trunc((now.getTime() - date.getTime()) / 1_000)
  const future = seconds < 0
  const distance = Math.abs(seconds)
  if (distance < 60) return { value: 0, unit: 'now', future }

  const units: Array<{ unit: Exclude<RelativeTimeUnit, 'now'>, seconds: number }> = [
    { unit: 'y', seconds: 365 * 24 * 60 * 60 },
    { unit: 'mo', seconds: 30 * 24 * 60 * 60 },
    { unit: 'd', seconds: 24 * 60 * 60 },
    { unit: 'h', seconds: 60 * 60 },
    { unit: 'm', seconds: 60 },
  ]
  const selected = units.find((candidate) => distance >= candidate.seconds) ?? units.at(-1)
  if (!selected) return undefined
  return {
    value: Math.max(1, Math.floor(distance / selected.seconds)),
    unit: selected.unit,
    future,
  }
}

export function visibleBlameWindow(
  visibleRanges: readonly ZeroBasedVisibleRange[],
  lineCount: number,
): OneBasedLineRange | undefined {
  if (!Number.isSafeInteger(lineCount) || lineCount <= 0) return undefined

  const valid = visibleRanges.filter((range) => (
    Number.isSafeInteger(range.startLine)
    && Number.isSafeInteger(range.endLine)
    && range.startLine >= 0
    && range.endLine >= range.startLine
  ))
  const first = valid[0]
  if (!first) {
    return { startLine: 1, endLine: Math.min(lineCount, MAX_BLAME_WINDOW_LINES) }
  }

  const start = Math.min(first.startLine, lineCount - 1)
  let end = Math.min(Math.max(...valid.map((range) => range.endLine)), lineCount - 1)
  if (end - start + 1 > MAX_BLAME_WINDOW_LINES) end = start + MAX_BLAME_WINDOW_LINES - 1
  return { startLine: start + 1, endLine: end + 1 }
}

export function blameableDocumentLineCount(documentLineCount: number, finalLineText: string): number {
  if (!Number.isSafeInteger(documentLineCount) || documentLineCount <= 0) return 0
  return finalLineText.length === 0 ? Math.max(0, documentLineCount - 1) : documentLineCount
}

export function revisionContentMatchesDocument(revisionContent: string, documentContent: string): boolean {
  return revisionContent === documentContent
}

export function normalizedTargetRanges(
  ranges: readonly TargetRange[],
  lineCount: number,
): TargetRange[] {
  if (!Number.isSafeInteger(lineCount) || lineCount <= 0) return []
  const normalized: TargetRange[] = []
  for (const range of ranges) {
    if (!Number.isSafeInteger(range.startLine) || !Number.isSafeInteger(range.endLine)) continue
    if (range.startLine < 1 || range.endLine < range.startLine) continue
    const startLine = Math.min(range.startLine, lineCount)
    const endLine = Math.min(range.endLine, lineCount)
    const previous = normalized.at(-1)
    if (previous && startLine <= previous.endLine + 1) {
      previous.endLine = Math.max(previous.endLine, endLine)
    } else {
      normalized.push({ startLine, endLine })
    }
  }
  return normalized
}

export function normalizedDeletionAnchors(
  anchors: readonly DeletionAnchor[],
  lineCount: number,
): DeletionAnchor[] {
  if (!Number.isSafeInteger(lineCount) || lineCount <= 0) return []
  const unique = new Set<number>()
  for (const anchor of anchors) {
    if (!Number.isSafeInteger(anchor.anchorLine)) continue
    unique.add(Math.min(Math.max(anchor.anchorLine, 1), lineCount))
  }
  return [...unique].sort((left, right) => left - right).map((anchorLine) => ({ anchorLine }))
}

export function traceFileHistorySelections(
  commits: readonly HistorySelectionCommit[],
  currentPath: string,
): HistorySelection[] {
  let tracedPath = currentPath
  return commits.map((commit) => {
    const changedFile = commit.files.find((file) => file.path === tracedPath)
      ?? commit.files.find((file) => file.oldPath === tracedPath)
    const selection = {
      commit: commit.commit,
      path: changedFile?.path ?? tracedPath,
      ...(changedFile?.oldPath ? { oldPath: changedFile.oldPath } : {}),
      ...(changedFile?.status ? { status: changedFile.status } : {}),
    }
    if (changedFile?.path === tracedPath && changedFile.oldPath) tracedPath = changedFile.oldPath
    return selection
  })
}
