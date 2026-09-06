import type { WorktreeInfo } from './types'

// Every worktree action is judged against the same three facts, so they travel
// together rather than being read off the repository at each call site.
export type WorktreeActionContext = {
  activeProjectRoot: string
  root: string
  defaultBranch: string
}

export type WorktreeBlocker = (worktree: WorktreeInfo, context: WorktreeActionContext) => string

export function worktreeName(worktree: WorktreeInfo): string {
  return worktree.branch || (worktree.detached ? 'detached' : worktreePathTail(worktree.path))
}

export function worktreePathTail(path: string): string {
  const cut = path.lastIndexOf('/')
  return cut >= 0 ? path.slice(cut + 1) : path
}

export function worktreeParentDirectory(path: string): string {
  const cut = path.lastIndexOf('/')
  return cut > 0 ? path.slice(0, cut) : ''
}

export function removalBlocker(worktree: WorktreeInfo, context: WorktreeActionContext): string {
  if (worktree.path === context.activeProjectRoot) return 'Main worktree'
  if (worktree.path === context.root) return 'Switch to Main first'
  if (worktree.detached || !worktree.branch) return 'Detached worktree'
  if (worktree.branch === context.defaultBranch) return 'Default branch'
  if (worktree.locked) return 'Locked worktree'
  if (worktree.dirty) return 'Local changes present'
  if (!worktree.merged_into_default) return `Not merged into ${context.defaultBranch}`
  return ''
}

// Moving relocates a checkout rather than deleting it, so merge state and the
// default branch do not matter. What matters is that nothing is holding the
// directory: the Service pins its repository handle to the viewed root, and a
// locked or dirty checkout must not be relocated out from under its work.
export function moveBlocker(worktree: WorktreeInfo, context: WorktreeActionContext): string {
  if (worktree.path === context.activeProjectRoot) return 'Main worktree'
  if (worktree.path === context.root) return 'Switch to Main first'
  if (worktree.prunable) return 'Missing from disk'
  if (worktree.locked) return 'Locked worktree'
  if (worktree.dirty) return 'Local changes present'
  return ''
}

// Sparse-checkout is editable on any checkout that is present and unlocked,
// including the main worktree, which is otherwise not selectable. GitGit only
// manages cone mode, so an existing non-cone selection is refused up front
// rather than failing on submit.
export function sparseBlocker(worktree: WorktreeInfo, context: WorktreeActionContext): string {
  void context
  if (worktree.bare) return 'Bare worktree'
  if (worktree.prunable) return 'Missing from disk'
  if (worktree.locked) return 'Locked worktree'
  if (!worktree.head) return 'Unborn HEAD'
  if (worktree.sparse.enabled && !worktree.sparse.cone) return 'Non-cone sparse checkout'
  return ''
}

export function selectedBlocker(
  worktrees: WorktreeInfo[],
  context: WorktreeActionContext,
  blocker: WorktreeBlocker,
  emptyReason = 'Select one or more worktrees',
): string {
  if (worktrees.length === 0) return emptyReason
  for (const worktree of worktrees) {
    const reason = blocker(worktree, context)
    if (reason) return `${worktreeName(worktree)}: ${reason}`
  }
  return ''
}

// An empty selection arrives as null from Go, so the count is read defensively
// rather than assumed to be an array.
export function sparseDirectories(worktree: WorktreeInfo): string[] {
  return worktree.sparse.directories ?? []
}

export function sparseSummary(worktree: WorktreeInfo): string {
  if (!worktree.sparse.enabled) return 'Full checkout'
  const count = sparseDirectories(worktree).length
  if (count === 0) return 'Sparse · root files only'
  return `Sparse · ${count} director${count === 1 ? 'y' : 'ies'}`
}

// A branch name is not a directory name. Slashes would create nesting the
// backend rejects, so they collapse to a dash.
export function defaultWorktreeName(branchOrRevision: string): string {
  return branchOrRevision.trim().replace(/[\\/]+/g, '-').replace(/^[-.]+/, '')
}

export type WorktreeDestination = { path: string; error: string }

// Mirrors the user-facing half of resolveWorktreeLocation so the dialog can
// disable its submit button as the user types. Advisory only: the backend
// re-validates, and it is the one that knows what exists on disk.
export function resolveWorktreeDestination(parentDirectory: string, name: string): WorktreeDestination {
  const parent = parentDirectory.trim().replace(/\/+$/, '')
  const trimmed = name.trim()
  if (!parent) return { path: '', error: 'Choose a parent directory.' }
  if (!parent.startsWith('/')) return { path: '', error: 'The parent directory must be an absolute path.' }
  if (!trimmed) return { path: '', error: 'Enter a worktree name.' }
  if (/[\\/]/.test(trimmed)) return { path: '', error: 'The name cannot contain a path separator.' }
  if (/[\r\n\0]/.test(trimmed)) return { path: '', error: 'The name cannot contain a newline.' }
  if (trimmed === '.' || trimmed === '..' || trimmed.startsWith('-')) {
    return { path: '', error: `“${trimmed}” is not a usable name.` }
  }
  return { path: `${parent}/${trimmed}`, error: '' }
}
