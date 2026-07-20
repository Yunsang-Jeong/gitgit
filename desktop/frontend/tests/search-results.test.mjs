import assert from 'node:assert/strict'
import test from 'node:test'
import { groupSearchResultsByCommit, searchResultCommitCount } from '../src/lib/search-results.ts'

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
  assert.deepEqual(grouped[0].matched_files.map((file) => file.path), ['one.go', 'two.go'])
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
