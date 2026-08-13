export type BranchNavigationKey = 'ArrowDown' | 'ArrowUp' | 'Home' | 'End' | 'PageDown' | 'PageUp'

export function orderedBranches(branches: string[], defaultBranch: string, currentBranch: string): string[] {
  const unique = Array.from(new Set(branches.filter(Boolean)))
  return unique.sort((left, right) => {
    const leftPriority = branchPriority(left, defaultBranch, currentBranch)
    const rightPriority = branchPriority(right, defaultBranch, currentBranch)
    return leftPriority - rightPriority || left.localeCompare(right, undefined, { numeric: true, sensitivity: 'base' })
  })
}

export function matchingBranches(branches: string[], query: string): string[] {
  const pattern = query.trim().toLocaleLowerCase()
  if (!pattern) return branches
  return branches
    .map((branch, index) => ({ branch, index, rank: matchRank(branch, pattern) }))
    .filter((candidate) => candidate.rank < 3)
    .sort((left, right) => left.rank - right.rank || left.index - right.index)
    .map((candidate) => candidate.branch)
}

export function nextVisibleBranchCount(current: number, total: number, pageSize = 25): number {
  return Math.min(total, Math.max(pageSize, current + pageSize))
}

export function nextBranchChoice(choices: string[], current: string, key: BranchNavigationKey, pageDistance = 10): string {
  if (choices.length === 0) return ''
  const currentIndex = Math.max(0, choices.indexOf(current))
  const distance = key === 'PageDown' || key === 'PageUp' ? pageDistance : 1
  const nextIndex = key === 'Home'
    ? 0
    : key === 'End'
      ? choices.length - 1
      : key === 'ArrowDown' || key === 'PageDown'
        ? Math.min(choices.length - 1, currentIndex + distance)
        : Math.max(0, currentIndex - distance)
  return choices[nextIndex]
}

export function worktreeHistoryScope(branch: string | undefined, detached = false): string {
  const normalized = branch?.trim() ?? ''
  return normalized && !detached ? normalized : 'HEAD'
}

function branchPriority(branch: string, defaultBranch: string, currentBranch: string): number {
  if (branch === defaultBranch) return 0
  if (branch === currentBranch) return 1
  return 2
}

function matchRank(branch: string, pattern: string): number {
  const normalized = branch.toLocaleLowerCase()
  if (normalized === pattern) return 0
  if (normalized.startsWith(pattern)) return 1
  return normalized.includes(pattern) ? 2 : 3
}
