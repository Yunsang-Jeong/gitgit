import assert from 'node:assert/strict'
import test from 'node:test'

import {
  buildSearchRequest,
  initialSearchState,
  reduceSearchState,
  type SearchDraft,
} from '../../src/search/model.js'
import type { SearchResponse, SearchResult } from '../../src/helper/protocol.js'

const draft: SearchDraft = {
  expression: ' MSG: *cache* OR FILE: **/*.go ',
  engine: 'glob',
  allRefs: false,
  author: ' Test ',
  since: ' last:30d ',
  until: '',
  followRename: true,
}

const result: SearchResult = {
  commit: 'a'.repeat(40),
  shortCommit: 'aaaaaaa',
  message: 'cache result',
  author: { name: 'Test' },
  date: '2026-08-13T00:00:00Z',
  refs: [],
  files: [{ status: 'M', path: 'main.go' }],
  matchSources: ['msg'],
}

function response(results = [result]): SearchResponse {
  return { scope: 'HEAD', allRefs: false, scanned: 42, count: results.length, results }
}

test('Search request preserves Desktop defaults and wire casing', () => {
  assert.deepEqual(buildSearchRequest('/repo', 7, draft, [
    { source: 'msg', value: '*cache*' },
    { source: 'file', value: '**/*.go', join: 'or', open_groups: 1, close_groups: 1 },
  ], new Date('2026-08-13T12:00:00.000Z')), {
    repositoryRoot: '/repo',
    requestId: 7,
    patterns: [
      { source: 'msg', value: '*cache*' },
      { source: 'file', value: '**/*.go', join: 'or', openGroups: 1, closeGroups: 1 },
    ],
    engine: 'glob',
    scope: 'HEAD',
    allRefs: false,
    author: 'Test',
    since: '2026-07-14T12:00:00.000Z',
    until: '',
    followRename: true,
    limit: 250,
    context: 3,
  })
})

test('new and failed requests preserve the previous successful results', () => {
  let state = initialSearchState()
  state = reduceSearchState(state, { type: 'edit', draft })
  state = reduceSearchState(state, { type: 'start', requestId: 1, draft })
  state = reduceSearchState(state, { type: 'success', requestId: 1, draft, response: response() })
  state = reduceSearchState(state, { type: 'start', requestId: 2, draft })
  assert.deepEqual(state.results, [result])
  state = reduceSearchState(state, { type: 'failure', requestId: 2, message: 'failed' })
  assert.equal(state.error, 'failed')
  assert.equal(state.notice, 'Previous successful results are preserved.')
  assert.deepEqual(state.results, [result])
})

test('stale progress/results are ignored and cancel discards partial state', () => {
  let state = reduceSearchState(initialSearchState(), { type: 'start', requestId: 4, draft })
  const unchanged = reduceSearchState(state, { type: 'progress', progress: { requestId: 3, scanned: 9, total: 10 } })
  assert.equal(unchanged, state)
  assert.equal(reduceSearchState(state, { type: 'success', requestId: 3, draft, response: response() }), state)
  state = reduceSearchState(state, { type: 'cancel', requestId: 4 })
  assert.equal(state.activeRequestId, undefined)
  assert.equal(state.notice, 'Search cancelled. Partial results were discarded.')
  assert.deepEqual(state.results, [])
})

test('editing after success marks the previous results stale', () => {
  let state = reduceSearchState(initialSearchState(), { type: 'start', requestId: 1, draft })
  state = reduceSearchState(state, { type: 'success', requestId: 1, draft, response: response() })
  state = reduceSearchState(state, { type: 'edit', draft: { ...draft, expression: 'FILE: **/*.ts' } })
  assert.equal(state.phase, 'stale')
  assert.deepEqual(state.results, [result])
})
