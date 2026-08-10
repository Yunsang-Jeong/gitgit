import assert from 'node:assert/strict'
import test from 'node:test'

import {
  matchingBranches,
  nextBranchChoice,
  nextVisibleBranchCount,
  orderedBranches,
  worktreeHistoryScope,
} from '../src/lib/branch-options.ts'
import {
  isRemoteBranchScope,
  remoteBranchLabel,
  remoteRelatedScope,
  remoteScopeUnavailable,
  visibleRemoteBranches,
} from '../src/lib/remote-branches.ts'

test('branch options keep default first, current second, and do not mutate input', () => {
  const branches = ['feature/zeta', 'release/2', 'main', 'feature/alpha', 'release/1', 'feature/zeta']
  assert.deepEqual(
    orderedBranches(branches, 'main', 'release/2'),
    ['main', 'release/2', 'feature/alpha', 'feature/zeta', 'release/1'],
  )
  assert.deepEqual(branches, ['feature/zeta', 'release/2', 'main', 'feature/alpha', 'release/1', 'feature/zeta'])
})

test('branch options preserve names that resemble natural-language scopes', () => {
  const branches = ['All', 'All branches', 'All refs', 'HEAD', 'detached', 'main']
  assert.deepEqual(orderedBranches(branches, 'main', 'All'), ['main', 'All', 'All branches', 'All refs', 'detached', 'HEAD'])
})

test('branch search ranks exact, prefix, then substring matches case-insensitively', () => {
  const branches = ['feature/search-ui', 'bugfix/search', 'search', 'Feature/Search-API', 'main']
  assert.deepEqual(
    matchingBranches(branches, 'SEARCH'),
    ['search', 'feature/search-ui', 'bugfix/search', 'Feature/Search-API'],
  )
  assert.deepEqual(matchingBranches(branches, '  search-api  '), ['Feature/Search-API'])
  assert.deepEqual(matchingBranches(branches, '한글'), [])
})

test('incremental branch reveal advances by a bounded page and stops at total', () => {
  assert.equal(nextVisibleBranchCount(25, 101), 50)
  assert.equal(nextVisibleBranchCount(50, 51), 51)
  assert.equal(nextVisibleBranchCount(51, 51), 51)
  assert.equal(nextVisibleBranchCount(0, 12), 12)
})

test('branch keyboard navigation reaches full-list boundaries without rendering every option', () => {
  const choices = ['All', ...Array.from({ length: 100 }, (_, index) => `branch-${String(index).padStart(3, '0')}`)]

  assert.equal(nextBranchChoice(choices, 'All', 'End'), 'branch-099')
  assert.equal(nextBranchChoice(choices, 'branch-099', 'Home'), 'All')
  assert.equal(nextBranchChoice(choices, 'branch-020', 'PageDown'), 'branch-030')
  assert.equal(nextBranchChoice(choices, 'branch-020', 'PageUp'), 'branch-010')
})

test('worktree selection follows an attached branch and uses HEAD when detached', () => {
  assert.equal(worktreeHistoryScope('feature/search-ui'), 'feature/search-ui')
  assert.equal(worktreeHistoryScope('detached'), 'detached')
  assert.equal(worktreeHistoryScope('detached', true), 'HEAD')
  assert.equal(worktreeHistoryScope(undefined), 'HEAD')
})

const remoteCatalog = [{
  remote: { name: 'origin', url: 'https://example.com/repo.git' },
  default_branch: 'main',
  count: 2,
  loaded: true,
  loading: false,
  branches: [
    { name: 'main', ref: 'refs/remotes/origin/main', default: true },
    { name: 'feature/search', ref: 'refs/remotes/origin/feature/search', default: false },
  ],
}]

test('remote branch helpers keep exact refs while presenting compact labels', () => {
  assert.equal(isRemoteBranchScope('refs/remotes/origin/feature/search'), true)
  assert.equal(isRemoteBranchScope('origin/feature/search'), false)
  assert.equal(remoteBranchLabel('refs/remotes/origin/feature/search'), 'origin/feature/search')
  assert.deepEqual(visibleRemoteBranches(remoteCatalog).map((branch) => branch.ref), [
    'refs/remotes/origin/main',
    'refs/remotes/origin/feature/search',
  ])
})

test('remote scope availability is confirmed only by a successful catalog read', () => {
  assert.equal(remoteScopeUnavailable('refs/remotes/origin/missing', remoteCatalog), true)
  assert.equal(remoteScopeUnavailable('refs/remotes/upstream/missing', remoteCatalog), false)
  assert.equal(remoteScopeUnavailable('refs/remotes/origin/missing', [{ ...remoteCatalog[0], loaded: false }]), false)
  assert.equal(remoteScopeUnavailable('refs/remotes/origin/missing', [{ ...remoteCatalog[0], error: 'read failed' }]), false)
})

test('remote side branch history relates to the same remote default ref', () => {
  assert.equal(remoteRelatedScope('refs/remotes/origin/feature/search', remoteCatalog), 'refs/remotes/origin/main')
  assert.equal(remoteRelatedScope('refs/remotes/origin/main', remoteCatalog), '')
})
