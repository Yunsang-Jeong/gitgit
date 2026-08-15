import type {
  SearchProgress,
  SearchRequest,
  SearchResponse,
  SearchResult,
} from '../helper/protocol.js'
import { parseSearchExpression, type SearchPattern } from './expression.js'

export type SearchEngine = 'glob' | 'regex'

export interface SearchDraft {
  expression: string
  engine: SearchEngine
  allRefs: boolean
  author: string
  since: string
  until: string
  followRename: boolean
}

export interface SearchViewState {
  draft: SearchDraft
  executedDraft?: SearchDraft
  activeRequestId?: number
  phase: 'idle' | 'running' | 'ready' | 'stale' | 'error'
  progress?: SearchProgress
  error: string
  notice: string
  results: SearchResult[]
  scanned: number
  hasMore: boolean
}

export type SearchStateAction =
  | { type: 'edit', draft: SearchDraft }
  | { type: 'start', requestId: number, draft: SearchDraft }
  | { type: 'progress', progress: SearchProgress }
  | { type: 'success', requestId: number, draft: SearchDraft, response: SearchResponse }
  | { type: 'failure', requestId: number, message: string }
  | { type: 'cancel', requestId: number }

export const DEFAULT_SEARCH_DRAFT: SearchDraft = {
  expression: '',
  engine: 'glob',
  allRefs: false,
  author: '',
  since: '',
  until: '',
  followRename: false,
}

export function initialSearchState(): SearchViewState {
  return {
    draft: { ...DEFAULT_SEARCH_DRAFT },
    phase: 'idle',
    error: '',
    notice: '',
    results: [],
    scanned: 0,
    hasMore: false,
  }
}

export function sameSearchDraft(left: SearchDraft, right: SearchDraft): boolean {
  return left.expression === right.expression
    && left.engine === right.engine
    && left.allRefs === right.allRefs
    && left.author === right.author
    && left.since === right.since
    && left.until === right.until
    && left.followRename === right.followRename
}

function settledPhase(state: SearchViewState, draft = state.draft): SearchViewState['phase'] {
  if (!state.executedDraft) return 'idle'
  return sameSearchDraft(state.executedDraft, draft) ? 'ready' : 'stale'
}

export function reduceSearchState(state: SearchViewState, action: SearchStateAction): SearchViewState {
  switch (action.type) {
    case 'edit':
      return {
        ...state,
        draft: { ...action.draft },
        phase: state.activeRequestId === undefined ? settledPhase(state, action.draft) : 'running',
        error: '',
        notice: state.executedDraft && !sameSearchDraft(state.executedDraft, action.draft)
          ? 'Expression changed. Run Search to refresh these results.'
          : '',
      }
    case 'start':
      return {
        ...state,
        activeRequestId: action.requestId,
        phase: 'running',
        progress: undefined,
        error: '',
        notice: '',
      }
    case 'progress':
      if (state.activeRequestId !== action.progress.requestId) return state
      return { ...state, progress: action.progress }
    case 'success':
      if (state.activeRequestId !== action.requestId) return state
      return {
        ...state,
        activeRequestId: undefined,
        executedDraft: { ...action.draft },
        phase: sameSearchDraft(action.draft, state.draft) ? 'ready' : 'stale',
        progress: undefined,
        error: '',
        notice: sameSearchDraft(action.draft, state.draft)
          ? ''
          : 'Expression changed while Search was running.',
        results: [...action.response.results],
        scanned: action.response.scanned,
        hasMore: action.response.hasMore,
      }
    case 'failure':
      if (state.activeRequestId !== action.requestId) return state
      return {
        ...state,
        activeRequestId: undefined,
        phase: 'error',
        progress: undefined,
        error: action.message,
        notice: state.results.length > 0 ? 'Previous successful results are preserved.' : '',
      }
    case 'cancel':
      if (state.activeRequestId !== action.requestId) return state
      return {
        ...state,
        activeRequestId: undefined,
        phase: settledPhase(state),
        progress: undefined,
        error: '',
        notice: 'Search cancelled. Partial results were discarded.',
      }
  }
}

export function buildSearchRequest(
  repositoryRoot: string,
  requestId: number,
  draft: SearchDraft,
  patterns: SearchPattern[],
  now = new Date(),
): SearchRequest {
  return {
    repositoryRoot,
    requestId,
    patterns: patterns.map((pattern) => ({
      source: pattern.source,
      value: pattern.value,
      ...(pattern.join ? { join: pattern.join } : {}),
      ...(pattern.open_groups ? { openGroups: pattern.open_groups } : {}),
      ...(pattern.close_groups ? { closeGroups: pattern.close_groups } : {}),
    })),
    engine: draft.engine,
    scope: draft.allRefs ? '' : 'HEAD',
    allRefs: draft.allRefs,
    author: draft.author.trim(),
    since: normalizeSearchBoundary(draft.since, 'since', now),
    until: normalizeSearchBoundary(draft.until, 'until', now),
    followRename: draft.followRename,
    limit: 250,
    context: 3,
  }
}

export function normalizeSearchBoundary(
  value: string,
  boundary: 'since' | 'until',
  now = new Date(),
): string {
  const normalized = value.trim()
  if (!normalized) return ''
  const relative = /^last:(\d+)d$/i.exec(normalized)
  if (relative) {
    const days = Number(relative[1])
    if (!Number.isFinite(days) || days < 0) return normalized
    const date = new Date(now)
    date.setDate(date.getDate() - days)
    return Number.isNaN(date.getTime()) ? normalized : date.toISOString()
  }
  const absolute = parseDisplayDate(normalized)
  if (!absolute) return normalized
  return (boundary === 'since' ? absolute.start : new Date(absolute.end.getTime() - 1)).toISOString()
}

function parseDisplayDate(value: string): { start: Date, end: Date } | undefined {
  const match = /^(\d{4})\.\s*(\d{1,2})\.\s*(\d{1,2})\.(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?)?$/u.exec(value)
  if (!match) return undefined
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const hour = match[4] === undefined ? 0 : Number(match[4])
  const minute = match[5] === undefined ? 0 : Number(match[5])
  const second = match[6] === undefined ? 0 : Number(match[6])
  const start = new Date(year, month - 1, day, hour, minute, second, 0)
  if (start.getFullYear() !== year || start.getMonth() !== month - 1 || start.getDate() !== day
    || start.getHours() !== hour || start.getMinutes() !== minute || start.getSeconds() !== second) return undefined
  const end = new Date(start)
  if (match[4] === undefined) end.setDate(end.getDate() + 1)
  else if (match[6] === undefined) end.setMinutes(end.getMinutes() + 1)
  else end.setSeconds(end.getSeconds() + 1)
  return { start, end }
}

export function validateSearchDraft(draft: SearchDraft): { patterns: SearchPattern[], error: string } {
  const parsed = parseSearchExpression(draft.expression)
  if (parsed.error) return parsed
  if (parsed.patterns.length === 0) return { patterns: [], error: 'Enter at least one MSG:, DIFF:, or FILE: condition.' }
  if (draft.followRename && !parsed.patterns.some((pattern) => pattern.source === 'file')) {
    return { patterns: parsed.patterns, error: 'Follow renames requires at least one FILE: condition.' }
  }
  return parsed
}
