import { createHash } from 'node:crypto'
import * as path from 'node:path'

import { isCommitObjectID, isSafeRepositoryRelativePath } from '../shared/repository.js'

export const REVISION_DOCUMENT_SCHEME = 'gitgit-revision'
const MAX_URI_PAYLOAD_BYTES = 32 * 1024

export type RevisionDocumentKind = 'target' | 'deleted-preimage'

export interface RevisionDocumentResource {
  v: 1
  kind: RevisionDocumentKind
  root: string
  contentRevision: string
  contentPath: string
  selectedCommit: string
  selectedPath: string
  selectedOldPath?: string
}

export interface RevisionDocumentUriParts {
  authority: string
  path: string
  query: string
  fragment?: string
}

export function revisionDocumentUriParts(resource: RevisionDocumentResource): RevisionDocumentUriParts {
  assertRevisionDocumentResource(resource)
  const payload = Buffer.from(JSON.stringify(resource), 'utf8')
  if (payload.byteLength > MAX_URI_PAYLOAD_BYTES) throw new Error('GitGit revision URI payload is too large.')
  return {
    authority: repositoryRootHash(resource.root),
    path: `/${resource.contentPath}`,
    query: payload.toString('base64url'),
  }
}

export function parseRevisionDocumentUriParts(parts: RevisionDocumentUriParts): RevisionDocumentResource {
  if (parts.fragment) throw new Error('GitGit revision URI fragments are not supported.')
  if (!/^[0-9a-f]{64}$/u.test(parts.authority)) throw new Error('GitGit revision URI has an invalid repository authority.')
  if (!parts.query || parts.query.length > MAX_URI_PAYLOAD_BYTES * 2 || !/^[A-Za-z0-9_-]+$/u.test(parts.query)) {
    throw new Error('GitGit revision URI has an invalid payload.')
  }

  let decoded: Buffer
  try {
    decoded = Buffer.from(parts.query, 'base64url')
  } catch {
    throw new Error('GitGit revision URI has an invalid payload.')
  }
  if (decoded.byteLength > MAX_URI_PAYLOAD_BYTES || decoded.toString('base64url') !== parts.query) {
    throw new Error('GitGit revision URI has a non-canonical payload.')
  }

  let value: unknown
  try {
    value = JSON.parse(decoded.toString('utf8'))
  } catch {
    throw new Error('GitGit revision URI payload is not valid JSON.')
  }
  assertRevisionDocumentResource(value)
  if (parts.authority !== repositoryRootHash(value.root)) {
    throw new Error('GitGit revision URI repository authority does not match its payload.')
  }
  if (parts.path !== `/${value.contentPath}`) {
    throw new Error('GitGit revision URI path does not match its payload.')
  }
  return value
}

export function revisionSelection(resource: RevisionDocumentResource): {
  commit: string
  path: string
  oldPath?: string
} {
  return {
    commit: resource.selectedCommit,
    path: resource.selectedPath,
    ...(resource.selectedOldPath ? { oldPath: resource.selectedOldPath } : {}),
  }
}

export function shouldUseDeletedPreimage(errorCode: string, parent: string, status?: string): boolean {
  return parent !== ''
    && status?.startsWith('D') === true
    && (errorCode === 'revision_content_not_found' || errorCode === 'revision_content_not_file')
}

export class RevisionRestoreGate {
  private suppressedUri: string | undefined

  suppress(uri: string | undefined): void {
    this.suppressedUri = uri
  }

  selectionStarted(): void {
    this.suppressedUri = undefined
  }

  allows(uri: string): boolean {
    if (this.suppressedUri === uri) return false
    this.suppressedUri = undefined
    return true
  }
}

function assertRevisionDocumentResource(value: unknown): asserts value is RevisionDocumentResource {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error('GitGit revision URI payload must be an object.')
  }
  const resource = value as Record<string, unknown>
  const allowed = new Set([
    'v',
    'kind',
    'root',
    'contentRevision',
    'contentPath',
    'selectedCommit',
    'selectedPath',
    'selectedOldPath',
  ])
  if (Object.keys(resource).some((key) => !allowed.has(key))) {
    throw new Error('GitGit revision URI payload contains unknown fields.')
  }
  if (resource.v !== 1 || (resource.kind !== 'target' && resource.kind !== 'deleted-preimage')) {
    throw new Error('GitGit revision URI payload has an unsupported version or kind.')
  }
  if (typeof resource.root !== 'string' || !isAbsolutePath(resource.root) || /[\0\n\r]/u.test(resource.root)) {
    throw new Error('GitGit revision URI payload has an invalid repository root.')
  }
  if (typeof resource.contentRevision !== 'string' || !isCommitObjectID(resource.contentRevision)
    || typeof resource.selectedCommit !== 'string' || !isCommitObjectID(resource.selectedCommit)) {
    throw new Error('GitGit revision URI payload has an invalid commit object ID.')
  }
  if (typeof resource.contentPath !== 'string' || !isSafeRepositoryRelativePath(resource.contentPath)
    || typeof resource.selectedPath !== 'string' || !isSafeRepositoryRelativePath(resource.selectedPath)) {
    throw new Error('GitGit revision URI payload has an invalid repository-relative path.')
  }
  if (resource.selectedOldPath !== undefined
    && (typeof resource.selectedOldPath !== 'string' || !isSafeRepositoryRelativePath(resource.selectedOldPath))) {
    throw new Error('GitGit revision URI payload has an invalid old path.')
  }
}

function isAbsolutePath(value: string): boolean {
  return path.posix.isAbsolute(value) || path.win32.isAbsolute(value)
}

function repositoryRootHash(root: string): string {
  return createHash('sha256').update(root, 'utf8').digest('hex')
}
