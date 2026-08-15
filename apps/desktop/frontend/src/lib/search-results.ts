import type { FileChange, PatternSource, SearchFileMatch, SearchResult } from './types'

export function groupSearchResultsByCommit(results: SearchResult[]): SearchResult[] {
  const grouped = new Map<string, SearchResult>()
  const canonicalMatches = new Set<string>()
  const canonicalChanges = new Set<string>()
  for (const result of results) {
    const existing = grouped.get(result.commit)
    if (!existing) {
      const hasCanonicalMatches = Array.isArray(result.matched_files)
      grouped.set(result.commit, {
        ...result,
        refs: result.refs ? [...result.refs] : undefined,
        files: [...result.files],
        changed_files: Array.isArray(result.changed_files)
          ? result.changed_files.map(cloneFile)
          : result.files.map(cloneFile),
        match_sources: [...result.match_sources],
        matched_files: hasCanonicalMatches
          ? result.matched_files.map(cloneMatchedFile)
          : legacyMatchedFiles(result),
      })
      if (hasCanonicalMatches) canonicalMatches.add(result.commit)
      if (Array.isArray(result.changed_files)) canonicalChanges.add(result.commit)
      continue
    }
    existing.match_sources = appendUnique(existing.match_sources, result.match_sources)
    if (!canonicalChanges.has(result.commit) && Array.isArray(result.changed_files)) {
      existing.changed_files = result.changed_files.map(cloneFile)
      canonicalChanges.add(result.commit)
    }
    if (canonicalMatches.has(result.commit)) continue
    if (Array.isArray(result.matched_files)) {
      existing.matched_files = result.matched_files.map(cloneMatchedFile)
      canonicalMatches.add(result.commit)
      continue
    }
    existing.matched_files = appendUniqueFiles(existing.matched_files, legacyMatchedFiles(result))
  }
  return [...grouped.values()]
}

export function searchResultCommitCount(results: SearchResult[]): number {
  return new Set(results.map((result) => result.commit)).size
}

export function formatSearchCommitCount(count: number, hasMore: boolean): string {
  return `${Number(count).toLocaleString()}${hasMore ? '+' : ''}`
}

function appendUnique(current: PatternSource[], incoming: PatternSource[]): PatternSource[] {
  const next = [...current]
  for (const source of incoming) {
    if (!next.includes(source)) next.push(source)
  }
  return next
}

function cloneFile(file: FileChange): FileChange {
  return { ...file }
}

function cloneMatchedFile(file: SearchFileMatch): SearchFileMatch {
  return { ...file, match_sources: [...file.match_sources] }
}

function legacyMatchedFiles(result: SearchResult): SearchFileMatch[] {
  if (!hasFileLevelMatch(result)) return []
  return [{
    ...cloneFile(result.file),
    match_sources: result.match_sources.filter((source): source is 'diff' | 'file' => source === 'diff' || source === 'file'),
  }]
}

function appendUniqueFiles(current: SearchFileMatch[], incoming: SearchFileMatch[]): SearchFileMatch[] {
  const next = [...current]
  for (const file of incoming) {
    if (!next.some((candidate) => fileKey(candidate) === fileKey(file))) next.push(file)
  }
  return next
}

function hasFileLevelMatch(result: SearchResult): boolean {
  return result.match_sources.some((source) => source === 'file' || source === 'diff')
}

function fileKey(file: FileChange): string {
  return `${file.status}\0${file.old_path ?? ''}\0${file.path}`
}
