import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_BLAME_WINDOW_LINES,
  blameAnnotation,
  blameableDocumentLineCount,
  compactRelativeTime,
  fileHistoryAccessibilityLabel,
  fileHistoryDescription,
  normalizedDeletionAnchors,
  normalizedTargetRanges,
  relativeTimeDescription,
  resolveBlameMode,
  isWorkingTreeCommit,
  revisionContentMatchesDocument,
  traceFileHistorySelections,
  visibleBlameWindow,
} from '../../src/editor/model.js'

test('blame annotation identifies GitGit and separates metadata from code', () => {
  const now = new Date('2026-08-15T12:00:00Z')
  assert.equal(
    blameAnnotation(
      '28af18e24a5082b64236fbe3d77e4d730c7e0917',
      'yunsang-jeong',
      '2026-08-13T11:59:00Z',
      now,
    ),
    '  │ GitGit · yunsang-jeong · 2d',
  )
  assert.equal(isWorkingTreeCommit('0'.repeat(40)), true)
  assert.equal(isWorkingTreeCommit('0'.repeat(64)), true)
  assert.equal(isWorkingTreeCommit(`${'0'.repeat(8)}${'a'.repeat(32)}`), false)
  assert.equal(
    blameAnnotation('0000000000000000000000000000000000000000', 'Not Committed Yet', '', now),
    '  │ GitGit · Working tree',
  )
  assert.equal(blameAnnotation('a'.repeat(40), 'Ada', 'not-a-date', now), '  │ GitGit · Ada')
})

test('relative time formatting is compact, deterministic, and screen-reader friendly', () => {
  const now = new Date('2026-08-15T12:00:00Z')
  const compactCases = [
    ['2026-08-15T11:59:31Z', 'now'],
    ['2026-08-15T11:58:30Z', '1m'],
    ['2026-08-15T09:30:00Z', '2h'],
    ['2026-08-13T11:59:00Z', '2d'],
    ['2026-06-15T12:00:00Z', '2mo'],
    ['2024-08-15T12:00:00Z', '2y'],
    ['2026-08-17T12:00:00Z', 'in 2d'],
  ] as const
  for (const [value, expected] of compactCases) {
    assert.equal(compactRelativeTime(value, now), expected)
  }
  assert.equal(relativeTimeDescription('2026-08-13T11:59:00Z', now), '2 days ago')
  assert.equal(relativeTimeDescription('2026-08-15T12:02:00Z', now), '2 minutes from now')
  assert.equal(compactRelativeTime('invalid', now), '')
  assert.equal(relativeTimeDescription('2026-08-15T11:00:00Z', new Date('invalid')), '')
})

test('file history presentation keeps the native row compact and supplies an action-oriented accessible label', () => {
  const now = new Date('2026-08-15T12:00:00Z')
  assert.equal(
    fileHistoryDescription('28af18e2', 'yunsang-jeong', '2026-08-13T11:59:00Z', now),
    'yunsang-jeong · 2d · 28af18e2',
  )
  assert.equal(
    fileHistoryDescription('28af18e2', 'yunsang-jeong', 'invalid', now),
    'yunsang-jeong · 28af18e2',
  )
  assert.equal(
    fileHistoryAccessibilityLabel(
      'feat: improve history',
      '28af18e2',
      'yunsang-jeong',
      '2026-08-13T11:59:00Z',
      now,
    ),
    'Commit feat: improve history. Authored by yunsang-jeong 2 days ago. Commit ID 28af18e2. Open read-only revision',
  )
})

test('auto blame only yields to the native Git blame surface', () => {
  assert.equal(resolveBlameMode('auto', false), 'activeLine')
  assert.equal(resolveBlameMode('auto', true), 'off')
  assert.equal(resolveBlameMode('visibleLines', true), 'visibleLines')
  assert.equal(resolveBlameMode('off', false), 'off')
})

test('file history selections follow a rename into older commits', () => {
  assert.deepEqual(traceFileHistorySelections([
    { commit: 'rename', files: [{ status: 'R100', path: 'new.ts', oldPath: 'old.ts' }] },
    { commit: 'modify', files: [{ status: 'M', path: 'other.ts' }, { status: 'M', path: 'old.ts' }] },
    { commit: 'root', files: [{ status: 'A', path: 'old.ts' }] },
  ], 'new.ts'), [
    { commit: 'rename', path: 'new.ts', oldPath: 'old.ts', status: 'R100' },
    { commit: 'modify', path: 'old.ts', status: 'M' },
    { commit: 'root', path: 'old.ts', status: 'A' },
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
