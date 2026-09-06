import assert from 'node:assert/strict'
import test from 'node:test'

import { commitDraftChangeKinds, commitDraftChangedIDs, commitDraftFingerprint, commitEditDisabledReason, deriveRewriteStackDraft, directionalDropInsertionIndex, directlyMovedCommitIDs, moveCommitTo, oldestAffectedOriginalIndex, projectVisualDraftToTargetChain, resolveEditTargetBranch, stabilizeDragPreview } from '../src/lib/commit-edit.ts'

const readyContext = {
  hasRepository: true,
  projectSwitching: false,
  worktreeSwitching: false,
}

test('Edit Mode can begin from the visible history without a selected commit or a resolved target branch', () => {
  assert.equal(commitEditDisabledReason(readyContext), '')
})

test('drag insertion preserves a permutation and accounts for removing the source row', () => {
  const commits = [{ commit: 'a' }, { commit: 'b' }, { commit: 'c' }, { commit: 'd' }]
  assert.deepEqual(moveCommitTo(commits, 'a', 3).map((commit) => commit.commit), ['b', 'c', 'a', 'd'])
  assert.deepEqual(moveCommitTo(commits, 'd', 0).map((commit) => commit.commit), ['d', 'a', 'b', 'c'])
  assert.deepEqual(moveCommitTo(commits, 'b', 2).map((commit) => commit.commit), ['a', 'b', 'c', 'd'])
  assert.deepEqual(moveCommitTo(commits, 'c', 4).map((commit) => commit.commit), ['a', 'b', 'd', 'c'])
})

test('drag preview crosses a target at a directional 30 percent overlap', () => {
  const target = { targetTop: 100, targetHeight: 100 }

  assert.equal(directionalDropInsertionIndex({ sourceIndex: 1, targetIndex: 2, pointerY: 129, ...target }), 2)
  assert.equal(directionalDropInsertionIndex({ sourceIndex: 1, targetIndex: 2, pointerY: 130, ...target }), 3)
  assert.equal(directionalDropInsertionIndex({ sourceIndex: 2, targetIndex: 1, pointerY: 171, ...target }), 2)
  assert.equal(directionalDropInsertionIndex({ sourceIndex: 2, targetIndex: 1, pointerY: 170, ...target }), 1)

  const commits = [{ commit: 'a' }, { commit: 'b' }, { commit: 'c' }, { commit: 'd' }]
  const insertionIndex = directionalDropInsertionIndex({ sourceIndex: 1, targetIndex: 2, pointerY: 130, ...target })
  assert.deepEqual(moveCommitTo(commits, 'b', insertionIndex).map((commit) => commit.commit), ['a', 'c', 'b', 'd'])
})

test('drag preview remains stable through reflow-driven candidate churn', () => {
  const entered = stabilizeDragPreview(null, 3, 130, 4)
  assert.deepEqual(entered, { insertionIndex: 3, anchorY: 130 })

  assert.equal(stabilizeDragPreview(entered, null, 131, 4), entered)
  assert.equal(stabilizeDragPreview(entered, 2, 128, 4), entered)
  assert.equal(stabilizeDragPreview(entered, 3, 133, 4), entered)

  assert.equal(stabilizeDragPreview(entered, null, 134, 4), null)
  assert.deepEqual(stabilizeDragPreview(entered, 4, 140, 4), { insertionIndex: 4, anchorY: 140 })
})

test('the moved treatment belongs to the directly dragged row, not every displaced row', () => {
  const original = [{ commit: 'a' }, { commit: 'b' }, { commit: 'c' }]
  const reordered = [{ commit: 'b' }, { commit: 'c' }, { commit: 'a' }]

  assert.deepEqual(directlyMovedCommitIDs(original, reordered, [], 'a'), ['a'])
  assert.deepEqual(directlyMovedCommitIDs(original, original, ['a'], 'a'), [])
})

test('message, author, and author-date edits identify replacement commits independently from row order', () => {
  const original = [
    { commit: 'a', message: 'first', author: { name: 'Alice', email: 'alice@example.com' }, date: '2026-01-01T00:00:00Z' },
    { commit: 'b', message: 'second', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-02T00:00:00Z' },
  ]
  const draft = [
    { commit: 'b', message: 'second revised', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-02T00:00:00Z' },
    { commit: 'a', message: 'first', author: { name: 'Alice Revised', email: 'alice+rewrite@example.com' }, date: '2026-01-01T01:30:00+01:30' },
  ]

  assert.deepEqual(commitDraftChangedIDs(original, draft), ['b', 'a'])
})

test('review identifies only the direct kinds of a changed commit', () => {
  const original = [{
    commit: 'a',
    message: 'original',
    author: { name: 'Alice', email: 'alice@example.com' },
    date: '2026-01-01T00:00:00Z',
  }]
  const changed = {
    commit: 'a',
    message: 'revised',
    author: { name: 'Alicia', email: 'alicia@example.com' },
    date: '2026-01-02T00:00:00Z',
  }

  assert.deepEqual(commitDraftChangeKinds(original, changed, ['a']), [
    'reordered',
    'message',
    'author',
    'author-date',
  ])
  assert.deepEqual(commitDraftChangeKinds(original, original[0], []), [])
})

test('draft fingerprint changes for order, message, author, and author-date changes only', () => {
  const original = [
    { commit: 'head', message: 'head', author: { name: 'Alice', email: 'alice@example.com' }, date: '2026-01-03T00:00:00Z', files: ['ignored'] },
    { commit: 'base', message: 'base', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-02T00:00:00Z' },
  ]
  const originalFingerprint = commitDraftFingerprint(original)

  assert.notEqual(commitDraftFingerprint([...original].reverse()), originalFingerprint)
  assert.notEqual(commitDraftFingerprint([{ ...original[0], message: 'revised' }, original[1]]), originalFingerprint)
  assert.notEqual(commitDraftFingerprint([{ ...original[0], author: { name: 'Alicia', email: 'alice@example.com' } }, original[1]]), originalFingerprint)
  assert.notEqual(commitDraftFingerprint([{ ...original[0], date: '2026-01-03T01:00:00Z' }, original[1]]), originalFingerprint)
  assert.equal(commitDraftFingerprint([{ ...original[0], files: ['also ignored'] }, original[1]]), originalFingerprint)
})

test('All branches resolves Review to the default branch instead of the retained visual scope', () => {
  assert.equal(resolveEditTargetBranch('feature/rewrite', true, 'main'), 'main')
  assert.equal(resolveEditTargetBranch('feature/rewrite', false, 'main'), 'feature/rewrite')
})

test('target-chain projection rejects a direct side-branch edit', () => {
  const side = { commit: 'side' }
  const head = { commit: 'main-head' }
  const middle = { commit: 'main-middle' }
  const anchor = { commit: 'main-anchor' }

  assert.deepEqual(projectVisualDraftToTargetChain(
    [side, head, middle, anchor],
    [side, head, middle, anchor],
    [head, middle, anchor],
    ['side'],
  ), {
    ok: false,
    reason: 'changed-commit-outside-target-chain',
    changedCommitIDs: ['side'],
  })
})

test('target-chain projection keeps interleaved side rows out and preserves the hidden tail order', () => {
  const sideNewest = { commit: 'side-newest', source: 'side' }
  const head = { commit: 'main-head', source: 'visible-original' }
  const sideMiddle = { commit: 'side-middle', source: 'side' }
  const middle = { commit: 'main-middle', source: 'visible-original' }
  const anchor = { commit: 'main-anchor', source: 'visible-original' }
  const hiddenTail = { commit: 'main-hidden-tail', source: 'hidden-original' }
  const original = [sideNewest, head, sideMiddle, middle, anchor]
  const draft = [sideNewest, { ...middle, source: 'draft' }, sideMiddle, { ...head, source: 'draft' }, anchor]

  const result = projectVisualDraftToTargetChain(
    original,
    draft,
    [head, middle, anchor, hiddenTail],
    ['main-middle'],
  )

  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.deepEqual(result.commits.map((commit) => commit.commit), [
    'main-middle',
    'main-head',
    'main-anchor',
    'main-hidden-tail',
  ])
  assert.equal(result.commits[0].source, 'draft')
  assert.equal(result.commits[1].source, 'draft')
  assert.equal(result.commits[3].source, 'hidden-original')
})

test('oldest affected original index anchors the complete rewrite range', () => {
  const original = [
    { commit: 'head', message: 'head', author: { name: 'Alice', email: 'alice@example.com' }, date: '2026-01-04T00:00:00Z' },
    { commit: 'middle', message: 'middle', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-03T00:00:00Z' },
    { commit: 'base', message: 'base', author: { name: 'Carol', email: 'carol@example.com' }, date: '2026-01-02T00:00:00Z' },
  ]

  assert.equal(oldestAffectedOriginalIndex(original, original), null)
  assert.equal(oldestAffectedOriginalIndex(original, [{ ...original[0], message: 'revised' }, original[1], original[2]]), 0)
  assert.equal(oldestAffectedOriginalIndex(original, [original[1], original[0], original[2]]), 1)
  assert.equal(oldestAffectedOriginalIndex(original, [original[0], original[1], { ...original[2], date: '2026-01-02T01:00:00Z' }]), 2)
  assert.equal(oldestAffectedOriginalIndex(original, [original[0], original[1], { ...original[2], commit: 'unknown' }]), null)
})

test('derives only an exact visible HEAD rewrite stack in backend oldest-first order', () => {
  const original = [
    { commit: 'head', message: 'head', author: { name: 'Alice', email: 'alice@example.com' }, date: '2026-01-04T00:00:00Z' },
    { commit: 'middle', message: 'middle', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-03T00:00:00Z' },
    { commit: 'anchor', message: 'anchor', author: { name: 'Carol', email: 'carol@example.com' }, date: '2026-01-02T00:00:00Z' },
    { commit: 'older', message: 'older', author: { name: 'Dana', email: 'dana@example.com' }, date: '2026-01-01T00:00:00Z' },
  ]
  const draft = [original[1], { ...original[0], message: 'head revised' }, original[2], original[3]]
  const result = deriveRewriteStackDraft(original, draft, [
    { commit: 'anchor' },
    { commit: 'middle' },
    { commit: 'head' },
  ])

  assert.deepEqual(result, {
    ok: true,
    commits: [original[2], { ...original[0], message: 'head revised' }, original[1]],
  })
})

test('rewrite stack derivation rejects filtered or cross-range visual drafts', () => {
  const original = [
    { commit: 'head', message: 'head', author: { name: 'Alice', email: 'alice@example.com' }, date: '2026-01-04T00:00:00Z' },
    { commit: 'middle', message: 'middle', author: { name: 'Bob', email: 'bob@example.com' }, date: '2026-01-03T00:00:00Z' },
    { commit: 'anchor', message: 'anchor', author: { name: 'Carol', email: 'carol@example.com' }, date: '2026-01-02T00:00:00Z' },
    { commit: 'older', message: 'older', author: { name: 'Dana', email: 'dana@example.com' }, date: '2026-01-01T00:00:00Z' },
  ]
  const stack = [{ commit: 'anchor' }, { commit: 'middle' }, { commit: 'head' }]

  assert.deepEqual(deriveRewriteStackDraft([original[3], ...original.slice(0, 3)], original, stack), {
    ok: false,
    reason: 'stack-is-not-visible-head-range',
  })
  assert.deepEqual(deriveRewriteStackDraft(original, [original[3], ...original.slice(0, 3)], stack), {
    ok: false,
    reason: 'draft-crosses-rewrite-range',
  })
  assert.deepEqual(deriveRewriteStackDraft(original, [original[0], original[1], original[2], { ...original[3], message: 'older revised' }], stack), {
    ok: false,
    reason: 'changes-outside-rewrite-range',
  })
})

test('commit editing remains disabled only while repository state is unavailable or transitioning', () => {
  assert.equal(commitEditDisabledReason({ ...readyContext, hasRepository: false }), 'Open a repository to edit its visible history.')
  assert.equal(commitEditDisabledReason({ ...readyContext, projectSwitching: true }), 'Wait for the project switch to finish.')
  assert.equal(commitEditDisabledReason({ ...readyContext, worktreeSwitching: true }), 'Wait for the worktree switch to finish.')
})

test('a commit changed only by a file edit is still a review anchor', () => {
  const commits = [
    { commit: 'c', message: 'third', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-03T00:00:00+09:00' },
    { commit: 'b', message: 'second', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-02T00:00:00+09:00' },
    { commit: 'a', message: 'first', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-01T00:00:00+09:00' },
  ]
  const fileEdits = new Map([['b', 'fingerprint-for-b']])

  assert.equal(oldestAffectedOriginalIndex(commits, commits), null)
  assert.equal(oldestAffectedOriginalIndex(commits, commits, fileEdits), 1)
  assert.deepEqual(commitDraftChangedIDs(commits, commits, fileEdits), ['b'])
  assert.deepEqual(commitDraftChangeKinds(commits, commits[1], [], fileEdits), ['files'])
  assert.notEqual(commitDraftFingerprint(commits, fileEdits), commitDraftFingerprint(commits))
})

test('a file edit outside the reviewed range is refused instead of being dropped', () => {
  const visible = [
    { commit: 'c', message: 'third', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-03T00:00:00+09:00' },
    { commit: 'b', message: 'second', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-02T00:00:00+09:00' },
    { commit: 'a', message: 'first', author: { name: 'A', email: 'a@example.com' }, date: '2026-01-01T00:00:00+09:00' },
  ]
  const stack = [{ commit: 'b' }, { commit: 'c' }]

  const withinRange = deriveRewriteStackDraft(visible, visible, stack, new Map([['b', 'edited']]))
  assert.equal(withinRange.ok, true)

  const outsideRange = deriveRewriteStackDraft(visible, visible, stack, new Map([['a', 'edited']]))
  assert.equal(outsideRange.ok, false)
  assert.equal(outsideRange.reason, 'changes-outside-rewrite-range')
})
