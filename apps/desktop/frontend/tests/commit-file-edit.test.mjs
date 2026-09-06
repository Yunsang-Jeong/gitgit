import assert from 'node:assert/strict'
import test from 'node:test'

import { createFileDraft, fileDraftDeletes, fileDraftFingerprint, fileDraftsToEdits, markFileDraftDeleted, restoreFileDraft, revertFileDraft, updateFileDraftContent } from '../src/lib/commit-file-edit.ts'

function changedFile(overrides = {}) {
  return {
    commit: 'c1',
    path: 'internal/app/search.go',
    content: 'package app\n',
    exists: true,
    editable: true,
    restorable: false,
    ...overrides,
  }
}

function deletedFile(overrides = {}) {
  return changedFile({
    path: 'docs/legacy.md',
    content: '',
    exists: false,
    restorable: true,
    restore_content: '# Legacy\n',
    ...overrides,
  })
}

test('a fresh draft records the original state and is not dirty', () => {
  const draft = createFileDraft(changedFile())
  assert.equal(draft.original_content, 'package app\n')
  assert.equal(draft.original_delete, false)
  assert.equal(draft.dirty, false)
  assert.equal(fileDraftDeletes(draft), false)
})

test('a commit that already deletes a path starts as an original deletion', () => {
  const draft = createFileDraft(deletedFile())
  assert.equal(draft.original_delete, true)
  assert.equal(draft.dirty, false)
  assert.equal(fileDraftDeletes(draft), true)
})

test('editing content marks the draft dirty and keeps the file present', () => {
  const draft = updateFileDraftContent(createFileDraft(changedFile()), 'package app // edited\n')
  assert.equal(draft.dirty, true)
  assert.equal(draft.exists, true)
  assert.equal(draft.content, 'package app // edited\n')
})

test('editing back to the original content clears dirty', () => {
  const edited = updateFileDraftContent(createFileDraft(changedFile()), 'changed\n')
  const restored = updateFileDraftContent(edited, 'package app\n')
  assert.equal(restored.dirty, false)
  assert.deepEqual(fileDraftsToEdits([restored]), [])
})

test('deleting a file the commit adds is dirty and drops its content', () => {
  const draft = markFileDraftDeleted(createFileDraft(changedFile()))
  assert.equal(draft.dirty, true)
  assert.equal(draft.exists, false)
  assert.equal(draft.content, '')
})

test('restoring a deleted path uses the parent content and is dirty', () => {
  const draft = restoreFileDraft(createFileDraft(deletedFile()))
  assert.equal(draft.dirty, true)
  assert.equal(draft.exists, true)
  assert.equal(draft.content, '# Legacy\n')
})

test('restore is refused for a path the commit does not delete', () => {
  const draft = createFileDraft(changedFile())
  assert.equal(restoreFileDraft(draft), draft)
})

test('reverting returns to the original content and delete state', () => {
  const deleted = markFileDraftDeleted(createFileDraft(changedFile()))
  const reverted = revertFileDraft(deleted)
  assert.equal(reverted.dirty, false)
  assert.equal(reverted.exists, true)
  assert.equal(reverted.content, 'package app\n')

  const restored = restoreFileDraft(createFileDraft(deletedFile()))
  const revertedDeletion = revertFileDraft(restored)
  assert.equal(revertedDeletion.dirty, false)
  assert.equal(revertedDeletion.exists, false)
})

test('only dirty drafts reach the rewrite payload, ordered by path', () => {
  const untouched = createFileDraft(changedFile({ path: 'a.go' }))
  const edited = updateFileDraftContent(createFileDraft(changedFile({ path: 'z.go' })), 'edited\n')
  const deleted = markFileDraftDeleted(createFileDraft(changedFile({ path: 'm.go' })))

  assert.deepEqual(fileDraftsToEdits([untouched, edited, deleted]), [
    { path: 'm.go', content: '', delete: true },
    { path: 'z.go', content: 'edited\n', delete: false },
  ])
})

test('the fingerprint is empty without changes and stable regardless of draft order', () => {
  const untouched = createFileDraft(changedFile())
  assert.equal(fileDraftFingerprint([untouched]), '')

  const edited = updateFileDraftContent(createFileDraft(changedFile({ path: 'z.go' })), 'edited\n')
  const deleted = markFileDraftDeleted(createFileDraft(changedFile({ path: 'm.go' })))
  assert.equal(fileDraftFingerprint([edited, deleted]), fileDraftFingerprint([deleted, edited]))
})

test('the fingerprint separates a deletion from an edit to empty content', () => {
  const deleted = markFileDraftDeleted(createFileDraft(changedFile()))
  const emptied = updateFileDraftContent(createFileDraft(changedFile()), '')
  assert.notEqual(fileDraftFingerprint([deleted]), fileDraftFingerprint([emptied]))
})
