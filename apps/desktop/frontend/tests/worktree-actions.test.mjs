import assert from 'node:assert/strict'
import test from 'node:test'

import { defaultWorktreeName, moveBlocker, removalBlocker, resolveWorktreeDestination, selectedBlocker, sparseBlocker, sparseSummary, worktreeName, worktreeParentDirectory, worktreePathTail } from '../src/lib/worktree-actions.ts'

const context = {
  activeProjectRoot: '/repo',
  root: '/repo',
  defaultBranch: 'main',
}

function worktree(overrides = {}) {
  return {
    path: '/work/feature',
    head: 'a'.repeat(40),
    branch: 'feature',
    detached: false,
    bare: false,
    locked: false,
    prunable: false,
    dirty: false,
    merged_into_default: true,
    sparse: { enabled: false, cone: false, directories: [] },
    ...overrides,
  }
}

test('removal keeps the order of reasons the grid used inline', () => {
  assert.equal(removalBlocker(worktree(), context), '')
  assert.equal(removalBlocker(worktree({ path: '/repo' }), context), 'Main worktree')
  assert.equal(
    removalBlocker(worktree({ path: '/other' }), { ...context, root: '/other' }),
    'Switch to Main first',
  )
  assert.equal(removalBlocker(worktree({ detached: true, branch: '' }), context), 'Detached worktree')
  assert.equal(removalBlocker(worktree({ branch: 'main' }), context), 'Default branch')
  assert.equal(removalBlocker(worktree({ locked: true }), context), 'Locked worktree')
  assert.equal(removalBlocker(worktree({ dirty: true }), context), 'Local changes present')
  assert.equal(removalBlocker(worktree({ merged_into_default: false }), context), 'Not merged into main')
})

test('a locked worktree is reported as locked before its local changes', () => {
  assert.equal(removalBlocker(worktree({ locked: true, dirty: true }), context), 'Locked worktree')
})

// Moving relocates rather than deletes, so merge state and the default branch
// are irrelevant to it.
test('move ignores merge state but refuses held checkouts', () => {
  assert.equal(moveBlocker(worktree({ merged_into_default: false }), context), '')
  assert.equal(moveBlocker(worktree({ branch: 'main' }), context), '')
  assert.equal(moveBlocker(worktree({ detached: true, branch: '' }), context), '')
  assert.equal(moveBlocker(worktree({ path: '/repo' }), context), 'Main worktree')
  assert.equal(moveBlocker(worktree({ prunable: true }), context), 'Missing from disk')
  assert.equal(moveBlocker(worktree({ locked: true }), context), 'Locked worktree')
  assert.equal(moveBlocker(worktree({ dirty: true }), context), 'Local changes present')
})

// The main worktree is not selectable in the grid, so sparse editing has to
// remain reachable for it or the first enable would be impossible there.
test('sparse editing is allowed on the main worktree and on a dirty one', () => {
  assert.equal(sparseBlocker(worktree({ path: '/repo' }), context), '')
  assert.equal(sparseBlocker(worktree({ dirty: true }), context), '')
  assert.equal(sparseBlocker(worktree({ bare: true }), context), 'Bare worktree')
  assert.equal(sparseBlocker(worktree({ prunable: true }), context), 'Missing from disk')
  assert.equal(sparseBlocker(worktree({ locked: true }), context), 'Locked worktree')
  assert.equal(sparseBlocker(worktree({ head: '' }), context), 'Unborn HEAD')
  assert.equal(
    sparseBlocker(worktree({ sparse: { enabled: true, cone: false, directories: ['a'] } }), context),
    'Non-cone sparse checkout',
  )
  assert.equal(
    sparseBlocker(worktree({ sparse: { enabled: true, cone: true, directories: ['a'] } }), context),
    '',
  )
})

test('a selection reports the first blocked worktree by name', () => {
  assert.equal(selectedBlocker([], context, removalBlocker), 'Select one or more worktrees')
  assert.equal(selectedBlocker([worktree()], context, removalBlocker), '')
  assert.equal(
    selectedBlocker([worktree(), worktree({ branch: 'wip', dirty: true })], context, removalBlocker),
    'wip: Local changes present',
  )
  assert.equal(
    selectedBlocker([worktree({ branch: '', detached: true, path: '/work/head' })], context, removalBlocker),
    'detached: Detached worktree',
  )
})

test('paths split into a name and a parent, including names with spaces', () => {
  assert.equal(worktreePathTail('/work/linked worktree'), 'linked worktree')
  assert.equal(worktreeParentDirectory('/work/linked worktree'), '/work')
  assert.equal(worktreePathTail('linked'), 'linked')
  assert.equal(worktreeParentDirectory('linked'), '')
  assert.equal(worktreeName(worktree({ branch: '', detached: true, path: '/work/x' })), 'detached')
})

test('sparse state is summarized for the badge', () => {
  assert.equal(sparseSummary(worktree()), 'Full checkout')
  assert.equal(sparseSummary(worktree({ sparse: { enabled: true, cone: true, directories: [] } })), 'Sparse · root files only')
  assert.equal(sparseSummary(worktree({ sparse: { enabled: true, cone: true, directories: ['a'] } })), 'Sparse · 1 directory')
  assert.equal(sparseSummary(worktree({ sparse: { enabled: true, cone: true, directories: ['a', 'b'] } })), 'Sparse · 2 directories')
})

test('a branch name becomes a usable directory name', () => {
  assert.equal(defaultWorktreeName('feature/foo'), 'feature-foo')
  assert.equal(defaultWorktreeName('  release/1.2  '), 'release-1.2')
  assert.equal(defaultWorktreeName('-weird'), 'weird')
})

test('the destination preview mirrors the backend rules', () => {
  assert.deepEqual(resolveWorktreeDestination('/work', 'linked'), { path: '/work/linked', error: '' })
  assert.deepEqual(resolveWorktreeDestination('/work/', 'linked'), { path: '/work/linked', error: '' })
  assert.equal(resolveWorktreeDestination('', 'linked').error, 'Choose a parent directory.')
  assert.equal(resolveWorktreeDestination('relative', 'linked').error, 'The parent directory must be an absolute path.')
  assert.equal(resolveWorktreeDestination('/work', '').error, 'Enter a worktree name.')
  assert.equal(resolveWorktreeDestination('/work', 'nested/linked').error, 'The name cannot contain a path separator.')
  assert.match(resolveWorktreeDestination('/work', '..').error, /not a usable name/)
  assert.match(resolveWorktreeDestination('/work', '--force').error, /not a usable name/)
  assert.equal(resolveWorktreeDestination('/work', 'linked worktree').path, '/work/linked worktree')
})
