import assert from 'node:assert/strict'
import test from 'node:test'

import {
  groupSearchPatternRange,
  removeSearchPatternAt,
  searchExpressionError,
  searchExpressionText,
  searchPatternDepths,
  searchPatternText,
  ungroupSearchPatternRange,
} from '../src/lib/search-expression.ts'

test('search expression validates balanced and nested parentheses', () => {
  assert.equal(searchExpressionError([
    { source: 'msg', value: 'one', open_groups: 2 },
    { source: 'file', value: 'two', join: 'or', close_groups: 1 },
    { source: 'diff', value: 'three', join: 'and', close_groups: 1 },
  ]), '')
  assert.match(searchExpressionError([
    { source: 'msg', value: 'one', close_groups: 1 },
  ]), /before its opening/)
  assert.match(searchExpressionError([
    { source: 'msg', value: 'one', open_groups: 1 },
  ]), /missing/)
})

test('removing a condition carries its group boundary to its neighbor', () => {
  assert.deepEqual(removeSearchPatternAt([
    { source: 'msg', value: 'one', open_groups: 1 },
    { source: 'file', value: 'two', join: 'or' },
    { source: 'diff', value: 'three', join: 'and', close_groups: 1 },
  ], 0), [
    { source: 'file', value: 'two', join: undefined, open_groups: 1 },
    { source: 'diff', value: 'three', join: 'and', close_groups: 1 },
  ])
})

test('search pattern text includes explicit group boundaries', () => {
  assert.equal(searchPatternText({ source: 'file', value: '**/*.go', join: 'and', open_groups: 1, close_groups: 2 }, 1), 'AND (FILE: **/*.go))')
})

test('grouping a selected range creates and removes one visual group', () => {
  const patterns = [
    { source: 'msg', value: '*cache*' },
    { source: 'file', value: '**/*.go', join: 'or' },
    { source: 'diff', value: '*context*', join: 'and' },
  ]
  const grouped = groupSearchPatternRange(patterns, 1, 2)

  assert.equal(searchExpressionText(grouped), 'MSG: *cache* OR (FILE: **/*.go AND DIFF: *context*)')
  assert.deepEqual(searchPatternDepths(grouped), [0, 1, 1])
  assert.deepEqual(ungroupSearchPatternRange(grouped, 1, 2), patterns)
})

test('group actions reject single, out-of-range, and unmatched selections', () => {
  const patterns = [
    { source: 'msg', value: 'one' },
    { source: 'diff', value: 'two', join: 'and' },
  ]

  assert.equal(groupSearchPatternRange(patterns, 0, 0), patterns)
  assert.equal(groupSearchPatternRange(patterns, -1, 1), patterns)
  assert.equal(ungroupSearchPatternRange(patterns, 0, 1), patterns)
})
