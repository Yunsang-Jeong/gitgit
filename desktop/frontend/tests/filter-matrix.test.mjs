import assert from 'node:assert/strict'
import test from 'node:test'

import {
  commitMatchesRule,
  isCommitHighlighted,
  isCommitVisible,
  matchesFilterValue,
  visibleCommits,
  visibleSearchResults,
} from '../src/lib/history.ts'
import {
  defaultFilterPresets,
  presetUnavailable,
  resolvePresetRules,
} from '../src/lib/presets.ts'

const baseCommit = {
  author: { name: 'Alice Example', email: 'alice@example.com' },
  message: 'feat: improve search history',
  date: '2026-07-18T12:00:00Z',
  branches: ['feature/search'],
  refs: ['HEAD -> feature/search'],
  files: [
    { status: 'R100', old_path: 'legacy/history.ts', path: 'src/lib/history.ts' },
  ],
}

const fieldCases = [
  { field: 'branch', hit: 'feature/*', miss: 'release/*' },
  { field: 'author', hit: 'alice@example.com', miss: 'bob@example.com' },
  { field: 'message', hit: '*search*', miss: '*worktree*' },
  { field: 'file', hit: 'src/**', miss: 'docs/**' },
  { field: 'date', hit: '2026-07', miss: '2025-01' },
]

test('filter fields and actions cover every 5 x 3 combination', () => {
  for (const { field, hit, miss } of fieldCases) {
    for (const action of ['highlight', 'hide', 'show']) {
      assert.equal(
        commitMatchesRule(baseCommit, { id: `${action}-${field}-hit`, action, field, pattern: hit }),
        true,
        `${action}/${field} should match`,
      )
      assert.equal(
        commitMatchesRule(baseCommit, { id: `${action}-${field}-miss`, action, field, pattern: miss }),
        false,
        `${action}/${field} should not match`,
      )
    }
  }
})

test('branch refs, author name, renamed path, date, and glob primitives are matched', () => {
  const cases = [
    ['HEAD -> feature/search', 'head -> feature/*', true],
    ['Alice Example', 'alice', true],
    ['legacy/history.ts', 'legacy/*', true],
    ['src/lib/history.ts', 'src/?ib/*', true],
    ['src/lib/history.ts', 'docs/**', false],
  ]
  for (const [value, pattern, expected] of cases) {
    assert.equal(matchesFilterValue(value, pattern), expected, `${pattern} against ${value}`)
  }

  assert.equal(commitMatchesRule(baseCommit, {
    id: 'old-path', action: 'show', field: 'file', pattern: 'legacy/*',
  }), true)

  const yesterday = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
  assert.equal(commitMatchesRule({ ...baseCommit, date: yesterday }, {
    id: 'recent', action: 'show', field: 'date', pattern: 'last:3d',
  }), true)
})

test('show/hide AND/OR truth table covers all 64 combinations', () => {
  const rules = [
    { id: 'show-a', action: 'show', field: 'message', pattern: 'show-a' },
    { id: 'show-b', action: 'show', field: 'message', pattern: 'show-b' },
    { id: 'hide-a', action: 'hide', field: 'message', pattern: 'hide-a' },
    { id: 'hide-b', action: 'hide', field: 'message', pattern: 'hide-b' },
  ]

  for (const show of ['and', 'or']) {
    for (const hide of ['and', 'or']) {
      for (let mask = 0; mask < 16; mask += 1) {
        const matches = [0, 1, 2, 3].map((bit) => Boolean(mask & (1 << bit)))
        const message = rules.filter((_, index) => matches[index]).map((rule) => rule.pattern).join(' ') || 'neutral'
        const shown = show === 'and' ? matches[0] && matches[1] : matches[0] || matches[1]
        const hidden = hide === 'and' ? matches[2] && matches[3] : matches[2] || matches[3]
        assert.equal(
          isCommitVisible({ ...baseCommit, message }, rules, { show, hide }),
          shown && !hidden,
          `show=${show} hide=${hide} mask=${mask.toString(2).padStart(4, '0')}`,
        )
      }
    }
  }
})

test('highlight is independent from visibility actions', () => {
  const highlight = { id: 'highlight', action: 'highlight', field: 'author', pattern: 'alice' }
  const show = { id: 'show', action: 'show', field: 'message', pattern: 'search' }
  const hide = { id: 'hide', action: 'hide', field: 'file', pattern: 'docs/**' }
  assert.equal(isCommitHighlighted(baseCommit, [highlight, show, hide]), true)
  assert.equal(isCommitVisible(baseCommit, [highlight, show, hide]), true)
  assert.equal(isCommitVisible(baseCommit, [highlight, { ...hide, pattern: 'src/**' }]), false)
})

test('the same filter rules are applied to commits and search results', () => {
  const rules = [
    { id: 'alice', action: 'show', field: 'author', pattern: 'alice' },
    { id: 'secret', action: 'hide', field: 'file', pattern: 'secret/**' },
  ]
  const visible = { ...baseCommit, commit: 'a', short_commit: 'a', parents: [] }
  const hidden = {
    ...baseCommit,
    commit: 'b',
    short_commit: 'b',
    parents: [],
    files: [{ status: 'M', path: 'secret/token.txt' }],
  }
  assert.deepEqual(visibleCommits([visible, hidden], rules).map((commit) => commit.commit), ['a'])

  const asResult = (commit) => ({
    ...commit,
    file: commit.files[0],
    diff: '',
    match_sources: ['msg'],
  })
  assert.deepEqual(visibleSearchResults([asResult(visible), asResult(hidden)], rules).map((result) => result.commit), ['a'])
})

test('preset activation covers none, each default, and both defaults', () => {
  const presets = defaultFilterPresets()
  const author = { name: 'Alice Example', email: 'alice@example.com' }
  const combinations = [
    { ids: [], expected: [] },
    { ids: ['my-jobs'], expected: ['Alice Example'] },
    { ids: ['3-days'], expected: ['last:3d'] },
    { ids: ['my-jobs', '3-days'], expected: ['Alice Example', 'last:3d'] },
  ]
  for (const { ids, expected } of combinations) {
    assert.deepEqual(resolvePresetRules(presets, ids, author).map((rule) => rule.pattern), expected)
  }
  assert.equal(presetUnavailable(presets[0], { name: '', email: '' }), true)
  assert.equal(presetUnavailable(presets[0], author), false)
})
