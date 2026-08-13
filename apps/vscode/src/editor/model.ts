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
  files: Array<{ path: string, oldPath?: string }>
}

export interface HistorySelection {
  commit: string
  path: string
  oldPath?: string
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
    }
    if (changedFile?.path === tracedPath && changedFile.oldPath) tracedPath = changedFile.oldPath
    return selection
  })
}
