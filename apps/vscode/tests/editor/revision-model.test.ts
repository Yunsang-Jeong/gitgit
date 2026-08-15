import assert from 'node:assert/strict'
import test from 'node:test'

import {
  parseRevisionDocumentUriParts,
  RevisionRestoreGate,
  revisionDocumentUriParts,
  revisionSelection,
  shouldUseDeletedPreimage,
  type RevisionDocumentResource,
} from '../../src/editor/revision-model.js'

const RESOURCE: RevisionDocumentResource = {
  v: 1,
  kind: 'target',
  root: '/Users/test/Repository with spaces',
  contentRevision: 'a'.repeat(40),
  contentPath: 'src/문서 #1?.ts',
  selectedCommit: 'a'.repeat(40),
  selectedPath: 'src/문서 #1?.ts',
  selectedOldPath: 'src/old name.ts',
}

test('revision document URI payload round-trips repository and selection metadata', () => {
  const parts = revisionDocumentUriParts(RESOURCE)
  assert.match(parts.authority, /^[0-9a-f]{64}$/u)
  assert.equal(parts.path, `/${RESOURCE.contentPath}`)
  assert.deepEqual(parseRevisionDocumentUriParts(parts), RESOURCE)

  const windows = { ...RESOURCE, root: 'C:\\work\\gitgit', contentRevision: 'b'.repeat(64) }
  assert.deepEqual(parseRevisionDocumentUriParts(revisionDocumentUriParts(windows)), windows)
})

test('revision document URI rejects tampering and unsafe metadata', () => {
  const parts = revisionDocumentUriParts(RESOURCE)
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, authority: '0'.repeat(64) }), /authority does not match/u)
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, path: '/other.ts' }), /path does not match/u)
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, fragment: 'line-1' }), /fragments/u)
  assert.throws(() => revisionDocumentUriParts({ ...RESOURCE, contentRevision: 'short' }), /commit object ID/u)
  assert.throws(() => revisionDocumentUriParts({ ...RESOURCE, selectedPath: '../secret' }), /repository-relative path/u)
  assert.throws(() => revisionDocumentUriParts({ ...RESOURCE, contentPath: '.git/config' }), /repository-relative path/u)
})

test('revision document URI rejects unknown versions and non-canonical payloads', () => {
  const parts = revisionDocumentUriParts(RESOURCE)
  const unknown = Buffer.from(JSON.stringify({ ...RESOURCE, v: 2 }), 'utf8').toString('base64url')
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, query: unknown }), /unsupported version/u)
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, query: `${parts.query}=` }), /invalid payload/u)
  const extra = Buffer.from(JSON.stringify({ ...RESOURCE, unexpected: true }), 'utf8').toString('base64url')
  assert.throws(() => parseRevisionDocumentUriParts({ ...parts, query: extra }), /unknown fields/u)
})

test('revision resource restores selection and deletion fallback semantics', () => {
  assert.deepEqual(revisionSelection(RESOURCE), {
    commit: RESOURCE.selectedCommit,
    path: RESOURCE.selectedPath,
    oldPath: RESOURCE.selectedOldPath,
  })
  assert.equal(shouldUseDeletedPreimage('revision_content_not_found', 'b'.repeat(40), 'D'), true)
  assert.equal(shouldUseDeletedPreimage('revision_content_not_file', 'b'.repeat(40), 'D100'), true)
  assert.equal(shouldUseDeletedPreimage('revision_content_not_file', 'b'.repeat(40), 'T'), false)
  assert.equal(shouldUseDeletedPreimage('revision_content_not_file', '', 'D'), false)
  assert.equal(shouldUseDeletedPreimage('binary_file', 'b'.repeat(40), 'D'), false)
})

test('user-cleared revision remains suppressed until a different selection starts', () => {
  const gate = new RevisionRestoreGate()
  gate.suppress('gitgit-revision://root/a')
  assert.equal(gate.allows('gitgit-revision://root/a'), false)
  assert.equal(gate.allows('gitgit-revision://root/a'), false)
  assert.equal(gate.allows('gitgit-revision://root/b'), true)

  gate.suppress('gitgit-revision://root/b')
  gate.selectionStarted()
  assert.equal(gate.allows('gitgit-revision://root/b'), true)
})
