import assert from 'node:assert/strict'
import test from 'node:test'

import { normalizeConeSelection, sparsePlan, sparseSelectionDiff } from '../src/lib/sparse-plan.ts'

const disabled = { enabled: false, cone: false, directories: [] }

function enabled(directories) {
  return { enabled: true, cone: true, directories }
}

test('a cone selection drops paths already covered by an ancestor', () => {
  assert.deepEqual(normalizeConeSelection(['a', 'a/b']), ['a'])
  assert.deepEqual(normalizeConeSelection(['a/b', 'a']), ['a'])
  assert.deepEqual(normalizeConeSelection(['a', 'b']), ['a', 'b'])
  // A shared prefix is not an ancestor.
  assert.deepEqual(normalizeConeSelection(['ab', 'a']), ['a', 'ab'])
  assert.deepEqual(normalizeConeSelection([' a/ ', '/a', 'a/']), ['a'])
  assert.deepEqual(normalizeConeSelection(['', '   ']), [])
})

test('the diff reports both directions', () => {
  assert.deepEqual(sparseSelectionDiff(['a', 'b'], ['b', 'c']), { added: ['c'], removed: ['a'] })
  assert.deepEqual(sparseSelectionDiff([], ['a']), { added: ['a'], removed: [] })
  assert.deepEqual(sparseSelectionDiff(['a'], ['a']), { added: [], removed: [] })
})

test('the plan truth table matches what the domain service does', () => {
  assert.equal(sparsePlan(disabled, ['a']).action, 'set')
  assert.equal(sparsePlan(disabled, []).action, 'none')
  assert.equal(sparsePlan(enabled(['a']), ['a']).action, 'none')
  assert.equal(sparsePlan(enabled(['a']), ['a', 'b']).action, 'expand')
  assert.equal(sparsePlan(enabled(['a', 'b']), ['a']).action, 'contract')
  assert.equal(sparsePlan(enabled(['a', 'b']), ['a', 'c']).action, 'set')
})

test('expand sends only the newly added directories', () => {
  const plan = sparsePlan(enabled(['a']), ['a', 'b', 'c'])
  assert.equal(plan.action, 'expand')
  assert.deepEqual(plan.directories, ['b', 'c'])
})

test('a mixed change sends the whole result set so it applies atomically', () => {
  const plan = sparsePlan(enabled(['a', 'b']), ['a', 'c'])
  assert.equal(plan.action, 'set')
  assert.deepEqual(plan.directories, ['a', 'c'])
  assert.deepEqual(plan.added, ['c'])
  assert.deepEqual(plan.removed, ['b'])
})

// SparseContract removes entries by exact string match against Git's own list,
// so a normalized picker value would silently miss and report the directory as
// absent.
test('contract names the strings Git reported, not the normalized ones', () => {
  const plan = sparsePlan(enabled(['a', 'a/b', 'c']), ['a'])
  assert.equal(plan.action, 'contract')
  assert.deepEqual(plan.directories, ['c'])

  const nested = sparsePlan(enabled(['docs', 'docs/guide', 'internal']), ['internal'])
  assert.equal(nested.action, 'contract')
  assert.deepEqual(nested.directories, ['docs', 'docs/guide'])
})

test('a null directory list from Go is treated as an empty selection', () => {
  assert.equal(sparsePlan({ enabled: true, cone: true, directories: undefined }, ['a']).action, 'expand')
  assert.equal(sparsePlan({ enabled: false, cone: false, directories: undefined }, []).action, 'none')
})
