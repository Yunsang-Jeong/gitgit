import type { Author, CommitFilterJoin, CommitFilterLogic, CommitFilterRule, CommitSummary, FileChange, SearchResult } from './types'
import { defaultFilterLogic } from './presets.ts'
import { formatDate, matchesDatePattern } from './datetime.ts'

type FilterableCommit = {
  author: Author
  message: string
  date: string
  files: FileChange[]
  branches?: string[]
  refs?: string[]
}

function escapeRegex(value: string): string {
  return value.replace(/[.+^${}()|[\]\\]/g, '\\$&')
}

export function matchesFilterValue(value: string, pattern: string): boolean {
  const normalizedValue = value.toLocaleLowerCase()
  const normalizedPattern = pattern.trim().toLocaleLowerCase()
  if (!normalizedPattern) return false
  if (!normalizedPattern.includes('*') && !normalizedPattern.includes('?')) {
    return normalizedValue.includes(normalizedPattern)
  }
  const expression = `^${escapeRegex(normalizedPattern).replaceAll('*', '.*').replaceAll('?', '.')}$`
  try {
    return new RegExp(expression, 'u').test(normalizedValue)
  } catch {
    return false
  }
}

export function commitMatchesRule(commit: FilterableCommit, rule: CommitFilterRule): boolean {
  switch (rule.field) {
    case 'branch':
      return [...(commit.branches ?? []), ...(commit.refs ?? [])].some((branch) => matchesFilterValue(branch, rule.pattern))
    case 'author':
      return [commit.author.name, commit.author.email].some((value) => matchesFilterValue(value, rule.pattern))
    case 'message':
      return matchesFilterValue(commit.message, rule.pattern)
    case 'file':
      return commit.files.some((file) => [file.old_path ?? '', file.path].some((value) => matchesFilterValue(value, rule.pattern)))
    case 'date':
      return matchesDateFilter(commit.date, rule.pattern)
  }
}

function matchesDateFilter(value: string, pattern: string, now = new Date()): boolean {
  const dateMatch = matchesDatePattern(value, pattern, now)
  if (dateMatch !== null) return dateMatch
  return [value, formatDate(value), formatDate(value, true)].some((candidate) => matchesFilterValue(candidate, pattern))
}

function combineMatches(matches: boolean[], join: CommitFilterJoin): boolean {
  return join === 'and' ? matches.every(Boolean) : matches.some(Boolean)
}

export function isCommitVisible(commit: FilterableCommit, rules: CommitFilterRule[], logic: CommitFilterLogic = defaultFilterLogic): boolean {
  const showRules = rules.filter((rule) => rule.action === 'show')
  const hideRules = rules.filter((rule) => rule.action === 'hide')
  const shown = showRules.length === 0 || combineMatches(showRules.map((rule) => commitMatchesRule(commit, rule)), logic.show)
  const hidden = hideRules.length > 0 && combineMatches(hideRules.map((rule) => commitMatchesRule(commit, rule)), logic.hide)
  return shown && !hidden
}

export function isCommitHighlighted(commit: FilterableCommit, rules: CommitFilterRule[]): boolean {
  return rules.some((rule) => rule.action === 'highlight' && commitMatchesRule(commit, rule))
}

export function visibleCommits(commits: CommitSummary[], rules: CommitFilterRule[], logic: CommitFilterLogic = defaultFilterLogic): CommitSummary[] {
  return commits.filter((commit) => isCommitVisible(commit, rules, logic))
}

export function visibleSearchResults(results: SearchResult[], rules: CommitFilterRule[], logic: CommitFilterLogic = defaultFilterLogic): SearchResult[] {
  return results.filter((result) => isCommitVisible(result, rules, logic))
}
