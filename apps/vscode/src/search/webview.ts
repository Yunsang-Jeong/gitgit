import type { SearchDraft } from './model.js'
import { isCommitObjectID, isSafeRepositoryRelativePath } from '../shared/repository.js'

export type SearchResultAction = 'selectCommitFile' | 'openFile'

export interface SearchResultSelection {
  action: SearchResultAction
  commit: string
  path: string
  oldPath?: string
}

export type WebviewIncomingMessage =
  | { type: 'draftChanged', draft: SearchDraft }
  | { type: 'search', draft: SearchDraft }
  | { type: 'cancel' }
  | SearchResultSelection

export function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/gu, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;',
  })[character] ?? character)
}

export function contentSecurityPolicy(nonce: string): string {
  if (!/^[A-Za-z0-9]{16,64}$/u.test(nonce)) throw new Error('Invalid Webview nonce.')
  return `default-src 'none'; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}';`
}

function record(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined
}

export function parseSearchDraft(value: unknown): SearchDraft | undefined {
  const draft = record(value)
  if (!draft) return undefined
  if (typeof draft.expression !== 'string' || draft.expression.length > 32_768) return undefined
  if (draft.engine !== 'glob' && draft.engine !== 'regex') return undefined
  if (typeof draft.allRefs !== 'boolean' || typeof draft.followRename !== 'boolean') return undefined
  if (typeof draft.author !== 'string' || draft.author.length > 1_024) return undefined
  if (typeof draft.since !== 'string' || draft.since.length > 256) return undefined
  if (typeof draft.until !== 'string' || draft.until.length > 256) return undefined
  return {
    expression: draft.expression,
    engine: draft.engine,
    allRefs: draft.allRefs,
    author: draft.author,
    since: draft.since,
    until: draft.until,
    followRename: draft.followRename,
  }
}

export function parseWebviewMessage(value: unknown): WebviewIncomingMessage | undefined {
  const message = record(value)
  if (!message || typeof message.type !== 'string') return undefined
  if (message.type === 'cancel') return { type: 'cancel' }
  if (message.type === 'draftChanged' || message.type === 'search') {
    const draft = parseSearchDraft(message.draft)
    return draft ? { type: message.type, draft } : undefined
  }
  if (message.type === 'selectCommitFile' || message.type === 'openFile') {
    if (typeof message.commit !== 'string' || !isCommitObjectID(message.commit)) return undefined
    if (typeof message.path !== 'string' || !isSafeRepositoryRelativePath(message.path)) return undefined
    if (message.oldPath !== undefined
      && (typeof message.oldPath !== 'string' || !isSafeRepositoryRelativePath(message.oldPath))) return undefined
    return {
      action: message.type,
      commit: message.commit,
      path: message.path,
      ...(typeof message.oldPath === 'string' ? { oldPath: message.oldPath } : {}),
    }
  }
  return undefined
}
