import assert from 'node:assert/strict'
import test from 'node:test'

import { buildHistoryDateRows, buildHistoryRowGeometry, historyDateSeparatorHeight } from '../src/lib/history-date-rows.ts'

function commit(commit, date) {
  return {
    author: { name: 'GitGit Test', email: 'gitgit@example.com' },
    commit,
    short_commit: commit,
    parents: [],
    message: commit,
    date,
    refs: [],
    files: [],
  }
}

function localDate(year, month, day) {
  return new Date(year, month - 1, day, 12).toISOString()
}

test('history uses daily, monthly, then yearly separators', () => {
  const rows = buildHistoryDateRows([
    commit('today', localDate(2026, 7, 22)),
    commit('today-again', localDate(2026, 7, 22)),
    commit('yesterday', localDate(2026, 7, 21)),
    commit('topology-rebound', localDate(2026, 7, 22)),
    commit('recent-edge', localDate(2026, 7, 16)),
    commit('older-this-month', localDate(2026, 7, 15)),
    commit('month-again', localDate(2026, 7, 1)),
    commit('previous-month', localDate(2026, 6, 30)),
    commit('previous-year', localDate(2025, 12, 31)),
    commit('same-previous-year', localDate(2025, 1, 1)),
  ], new Date(2026, 6, 22, 12))

  assert.deepEqual(rows.map((row) => row.separator?.label ?? null), [
    '2026. 7. 22.',
    null,
    '2026. 7. 21.',
    null,
    '2026. 7. 16.',
    '2026. 7. 1.',
    null,
    '2026. 6. 1.',
    '2025. 1. 1.',
    null,
  ])
})

test('history row geometry reserves separator height before commit rows', () => {
  const rows = buildHistoryDateRows([
    commit('first', localDate(2026, 7, 22)),
    commit('second', localDate(2026, 7, 22)),
    commit('third', localDate(2026, 7, 21)),
  ], new Date(2026, 6, 22, 12))
  const geometry = buildHistoryRowGeometry(rows, 32)

  assert.equal(historyDateSeparatorHeight, 22)
  assert.deepEqual([...geometry.tops.entries()], [['first', 22], ['second', 54], ['third', 108]])
  assert.equal(geometry.height, 140)
})
