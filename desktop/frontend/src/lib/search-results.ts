import type { FileChange, PatternSource, SearchResult } from './types'

export function groupSearchResultsByCommit(results: SearchResult[]): SearchResult[] {
  const grouped = new Map<string, SearchResult>()
  for (const result of results) {
    const existing = grouped.get(result.commit)
    if (!existing) {
      grouped.set(result.commit, {
        ...result,
        refs: result.refs ? [...result.refs] : undefined,
        files: [...result.files],
        match_sources: [...result.match_sources],
        matched_files: [cloneFile(result.file)],
      })
      continue
    }
    existing.match_sources = appendUnique(existing.match_sources, result.match_sources)
    if (!existing.matched_files?.some((file) => fileKey(file) === fileKey(result.file))) {
      existing.matched_files = [...(existing.matched_files ?? []), cloneFile(result.file)]
    }
  }
  return [...grouped.values()]
}

export function searchResultCommitCount(results: SearchResult[]): number {
  return new Set(results.map((result) => result.commit)).size
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

function fileKey(file: FileChange): string {
  return `${file.status}\0${file.old_path ?? ''}\0${file.path}`
}
