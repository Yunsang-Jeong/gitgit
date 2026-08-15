import assert from 'node:assert/strict'
import test from 'node:test'

import type { SearchResult } from '../../src/helper/protocol.js'
import {
  commitSubject,
  formatRelativeDate,
  matchSummary,
  searchFilterSummary,
  searchFileSelection,
  searchStatusMessage,
  searchTree,
} from '../../src/search/tree-model.js'

const result: SearchResult = {
  commit: 'a'.repeat(40),
  shortCommit: 'aaaaaaa',
  message: 'feat: keep the sidebar compact\n\nDetails',
  author: { name: 'Test Author' },
  date: '2026-08-12T12:00:00Z',
  refs: ['main', 'origin/main'],
  matchedFiles: [
    { status: 'M', path: 'apps/vscode/src/search/view.ts', matchSources: ['diff', 'file'] },
  ],
  changedFiles: [
    { status: 'M', path: 'apps/vscode/src/search/view.ts' },
    { status: 'A', path: 'apps/vscode/tests/search/view.test.ts' },
  ],
  matchSources: ['msg', 'diff', 'file'],
}

test('commit-first tree separates matched files from other changed files', () => {
  const commit = searchTree([result], new Date('2026-08-15T12:00:00Z'))[0]!
  assert.equal(commit.label, 'feat: keep the sidebar compact')
  assert.equal(commit.description, '3d · Test Author · main · 1 matched · 2 changed')
  assert.deepEqual(commit.children.map((group) => [group.label, group.description]), [
    ['Matched files', '1'],
    ['Other changed files', '1'],
  ])
  assert.equal(commit.children[0]!.children[0]!.label, 'view.ts')
  assert.equal(commit.children[0]!.children[0]!.description, 'M · apps/vscode/src/search · Diff + Path')
  assert.equal(commit.children[1]!.children[0]!.label, 'view.test.ts')
})

test('message-only empty commit remains a visible leaf', () => {
  const empty: SearchResult = {
    ...result,
    commit: 'b'.repeat(40),
    matchedFiles: [],
    changedFiles: [],
    matchSources: ['msg'],
  }
  const commit = searchTree([empty], new Date('2026-08-15T12:00:00Z'))[0]!
  assert.equal(matchSummary(empty), 'Message match · 0 changed')
  assert.deepEqual(commit.children, [])
})

test('message matches expose all changes once without claiming file matches', () => {
  const messageMatch: SearchResult = {
    ...result,
    matchedFiles: [],
    matchSources: ['msg'],
  }
  const commit = searchTree([messageMatch], new Date('2026-08-15T12:00:00Z'))[0]!
  assert.deepEqual(commit.children.map((group) => group.label), ['Changed files'])
  assert.equal(commit.children[0]!.children.length, 2)
})

test('relative dates and subjects fail quietly for unusual metadata', () => {
  assert.equal(formatRelativeDate('2026-08-15T11:59:40Z', new Date('2026-08-15T12:00:00Z')), 'now')
  assert.equal(formatRelativeDate('2026-06-01T12:00:00Z', new Date('2026-08-15T12:00:00Z')), '2mo')
  assert.equal(formatRelativeDate('2025-08-16T12:00:00Z', new Date('2026-08-15T12:00:00Z')), '12mo')
  assert.equal(formatRelativeDate('2025-08-15T12:00:00Z', new Date('2026-08-15T12:00:00Z')), '1y')
  assert.equal(formatRelativeDate('not-a-date'), 'unknown date')
  assert.equal(formatRelativeDate('2026-08-16T00:00:00Z', new Date('2026-08-15T12:00:00Z')), 'in the future')
  assert.equal(commitSubject('\n'), '(no commit message)')
})

test('filter summary describes active native QuickInput filters', () => {
  assert.equal(searchFilterSummary({
    expression: 'MSG: *cache*',
    engine: 'regex',
    allRefs: true,
    author: ' Test ',
    since: 'last:30d',
    until: '2026. 08. 15.',
    followRename: true,
  }), 'All refs · Regex · Author: Test · Since: last:30d · Until: 2026. 08. 15. · Follow renames')
})

test('status keeps preserved-result failure and cancellation notices visible', () => {
  const ready = {
    draft: {
      expression: 'MSG: *cache*',
      engine: 'glob' as const,
      allRefs: false,
      author: '',
      since: '',
      until: '',
      followRename: false,
    },
    phase: 'ready' as const,
    error: '',
    notice: 'Search cancelled. Partial results were discarded.',
    results: [result],
    scanned: 42,
    hasMore: false,
  }
  assert.equal(
    searchStatusMessage(ready, { phase: 'ready', message: '' }),
    '1 commits · 42 scanned · HEAD · Glob · Search cancelled. Partial results were discarded.',
  )
  assert.equal(
    searchStatusMessage({
      ...ready,
      phase: 'error',
      error: 'Search failed.',
      notice: 'Previous successful results are preserved.',
    }, { phase: 'ready', message: '' }),
    'Search failed. · Previous successful results are preserved.',
  )
})

test('file actions accept native tree nodes and reject unsafe command arguments', () => {
  const commit = searchTree([result])[0]!
  assert.deepEqual(searchFileSelection(commit.children[0]!.children[0]!), {
    commit: 'a'.repeat(40),
    path: 'apps/vscode/src/search/view.ts',
    status: 'M',
  })
  assert.equal(searchFileSelection({ commit: 'bad', path: 'view.ts' }), undefined)
  assert.equal(searchFileSelection({ commit: 'a'.repeat(40), path: '../secret' }), undefined)
  assert.equal(searchFileSelection({ commit: 'a'.repeat(40), path: 'src\\secret.ts' }), undefined)
  assert.equal(searchFileSelection({ commit: 'a'.repeat(40), path: '.git/config' }), undefined)
  assert.equal(searchFileSelection({ commit: 'a'.repeat(40), path: 'view.ts', status: '<script>' }), undefined)
})
