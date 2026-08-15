import assert from 'node:assert/strict'
import test from 'node:test'
import { formatSearchCommitCount, groupSearchResultsByCommit, searchResultCommitCount } from '../src/lib/search-results.ts'

function result(commit, path, sources) {
  return {
    author: { name: 'Example', email: 'example@example.com' },
    commit,
    short_commit: commit.slice(0, 8),
    message: `message ${commit}`,
    date: '2026-07-20T00:00:00Z',
    file: { status: 'M', path },
    files: [{ status: 'M', path }],
    diff: '',
    match_sources: sources,
  }
}

test('search results collapse file matches into one row per commit', () => {
  const grouped = groupSearchResultsByCommit([
    result('aaaaaaaa', 'one.go', ['msg']),
    result('aaaaaaaa', 'two.go', ['file']),
    result('bbbbbbbb', 'README.md', ['diff']),
  ])

  assert.equal(grouped.length, 2)
  assert.deepEqual(grouped[0].matched_files.map((file) => file.path), ['two.go'])
  assert.deepEqual(grouped[0].match_sources, ['msg', 'file'])
  assert.equal(searchResultCommitCount(grouped), 2)
})

test('duplicate file matches are only represented once', () => {
  const grouped = groupSearchResultsByCommit([
    result('aaaaaaaa', 'one.go', ['msg']),
    result('aaaaaaaa', 'one.go', ['diff']),
  ])

  assert.equal(grouped[0].matched_files.length, 1)
  assert.deepEqual(grouped[0].match_sources, ['msg', 'diff'])
})

test('message-only matches do not label every changed file as a file match', () => {
  const grouped = groupSearchResultsByCommit([
    result('aaaaaaaa', 'one.go', ['msg']),
    result('aaaaaaaa', 'two.go', ['msg']),
  ])

  assert.deepEqual(grouped[0].matched_files, [])
})

test('canonical matched and changed files are preserved instead of rebuilt from the legacy row', () => {
  const input = {
    ...result('aaaaaaaa', 'legacy-row.go', ['diff', 'file']),
    matched_files: [
      { status: 'M', path: 'one.go', match_sources: ['diff'] },
      { status: 'A', path: 'two.go', match_sources: ['file'] },
    ],
    changed_files: [
      { status: 'M', path: 'one.go' },
      { status: 'A', path: 'two.go' },
      { status: 'M', path: 'other.go' },
    ],
  }

  const [grouped] = groupSearchResultsByCommit([input])
  assert.deepEqual(grouped.matched_files, input.matched_files)
  assert.deepEqual(grouped.changed_files, input.changed_files)
  assert.notEqual(grouped.matched_files, input.matched_files)
  assert.notEqual(grouped.matched_files[0].match_sources, input.matched_files[0].match_sources)
})

test('canonical matched files supersede a preceding legacy row for the same commit', () => {
  const canonical = {
    ...result('aaaaaaaa', 'legacy-placeholder.go', ['file']),
    matched_files: [{ status: 'M', path: 'canonical.go', match_sources: ['file'] }],
    changed_files: [{ status: 'M', path: 'canonical.go' }],
  }
  const grouped = groupSearchResultsByCommit([
    result('aaaaaaaa', 'legacy.go', ['file']),
    canonical,
  ])

  assert.deepEqual(grouped[0].matched_files, canonical.matched_files)
  assert.deepEqual(grouped[0].changed_files, canonical.changed_files)
})

test('truncated commit counts use an explicit plus suffix', () => {
  assert.equal(formatSearchCommitCount(250, true), '250+')
  assert.equal(formatSearchCommitCount(12, false), '12')
})
