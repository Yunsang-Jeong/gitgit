import assert from 'node:assert/strict'
import test from 'node:test'
import { formatDate, matchesDatePattern, normalizeSearchBoundary } from '../src/lib/datetime.ts'

test('dates use the GitGit calendar format with optional time', () => {
  const local = new Date(2026, 6, 19, 9, 8, 7)
  assert.equal(formatDate(local.toISOString()), '2026. 7. 19.')
  assert.equal(formatDate(local.toISOString(), true), '2026. 7. 19. 09:08:07')
})

test('date filters support relative, calendar-day, minute, and second precision', () => {
  const now = new Date(2026, 6, 19, 12, 0, 0)
  assert.equal(matchesDatePattern(new Date(2026, 6, 17, 12, 0, 0).toISOString(), 'last:3d', now), true)
  assert.equal(matchesDatePattern(new Date(2026, 6, 15, 12, 0, 0).toISOString(), 'last:3d', now), false)
  assert.equal(matchesDatePattern(new Date(2026, 6, 19, 23, 59, 59).toISOString(), '2026. 7. 19.'), true)
  assert.equal(matchesDatePattern(new Date(2026, 6, 19, 9, 30, 45).toISOString(), '2026. 7. 19. 09:30'), true)
  assert.equal(matchesDatePattern(new Date(2026, 6, 19, 9, 30, 45).toISOString(), '2026. 7. 19. 09:30:45'), true)
})

test('search boundaries normalize display dates and last:N days to ISO timestamps', () => {
  const now = new Date(2026, 6, 19, 12, 0, 0)
  assert.equal(normalizeSearchBoundary('2026. 7. 19.', 'since'), new Date(2026, 6, 19, 0, 0, 0).toISOString())
  assert.equal(normalizeSearchBoundary('2026. 7. 19.', 'until'), new Date(2026, 6, 19, 23, 59, 59, 999).toISOString())
  assert.equal(normalizeSearchBoundary('last:3d', 'since', now), new Date(2026, 6, 16, 12, 0, 0).toISOString())
})
