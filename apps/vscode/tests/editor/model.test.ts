import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_BLAME_WINDOW_LINES,
  blameableDocumentLineCount,
  normalizedDeletionAnchors,
  normalizedTargetRanges,
  revisionContentMatchesDocument,
  traceFileHistorySelections,
  visibleBlameWindow,
} from '../../src/editor/model.js'

test('file history selections follow a rename into older commits', () => {
  assert.deepEqual(traceFileHistorySelections([
    { commit: 'rename', files: [{ path: 'new.ts', oldPath: 'old.ts' }] },
    { commit: 'modify', files: [{ path: 'other.ts' }, { path: 'old.ts' }] },
    { commit: 'root', files: [{ path: 'old.ts' }] },
  ], 'new.ts'), [
    { commit: 'rename', path: 'new.ts', oldPath: 'old.ts' },
    { commit: 'modify', path: 'old.ts' },
    { commit: 'root', path: 'old.ts' },
  ])
})

test('visibleBlameWindow converts visible ranges to a bounded 1-based range', () => {
  assert.deepEqual(visibleBlameWindow([{ startLine: 8, endLine: 19 }], 100), {
    startLine: 9,
    endLine: 20,
  })
  assert.deepEqual(visibleBlameWindow([], 3_000), {
    startLine: 1,
    endLine: MAX_BLAME_WINDOW_LINES,
  })
  assert.deepEqual(visibleBlameWindow([{ startLine: 10, endLine: 2_900 }], 3_000), {
    startLine: 11,
    endLine: 2_010,
  })
  assert.equal(visibleBlameWindow([], 0), undefined)
})

test('blame excludes VS Code trailing-newline and empty-document phantom lines', () => {
  assert.equal(blameableDocumentLineCount(2, 'second'), 2)
  assert.equal(blameableDocumentLineCount(3, ''), 2)
  assert.equal(blameableDocumentLineCount(2, ''), 1)
  assert.equal(blameableDocumentLineCount(1, ''), 0)
})

test('commit ranges are safe only when revision and editor content are identical', () => {
  assert.equal(revisionContentMatchesDocument('first\nsecond\n', 'first\nsecond\n'), true)
  assert.equal(revisionContentMatchesDocument('first\nsecond\n', 'inserted\nfirst\nsecond\n'), false)
  assert.equal(revisionContentMatchesDocument('first\nsecond\n', 'first\r\nsecond\r\n'), false)
  assert.equal(revisionContentMatchesDocument('first\nsecond\n', 'first\nsecond'), false)
})

test('diff ranges and deletion anchors are clamped, merged, and deduplicated', () => {
  assert.deepEqual(normalizedTargetRanges([
    { startLine: 2, endLine: 4 },
    { startLine: 5, endLine: 8 },
    { startLine: 12, endLine: 30 },
    { startLine: 0, endLine: 1 },
  ], 20), [
    { startLine: 2, endLine: 8 },
    { startLine: 12, endLine: 20 },
  ])
  assert.deepEqual(normalizedDeletionAnchors([
    { anchorLine: 0 },
    { anchorLine: 4 },
    { anchorLine: 4 },
    { anchorLine: 99 },
  ], 10), [
    { anchorLine: 1 },
    { anchorLine: 4 },
    { anchorLine: 10 },
  ])
})
