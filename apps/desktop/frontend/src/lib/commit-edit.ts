export type CommitEditContext = {
  hasRepository: boolean
  projectSwitching: boolean
  worktreeSwitching: boolean
}

export type CommitDraft = {
  commit: string
  message: string
  author: { name: string; email: string }
  date: string
}

export type CommitDraftChangeKind = 'reordered' | 'message' | 'author' | 'author-date' | 'files'

// File edits are drafted per commit outside the commit rows, so every
// comparison that answers "did this commit change?" has to be handed the file
// draft fingerprints as well. A commit missing from the map has no file edits.
export type CommitFileFingerprints = ReadonlyMap<string, string>

const noFileFingerprints: CommitFileFingerprints = new Map()

function fileFingerprintOf(fileFingerprints: CommitFileFingerprints, commitID: string): string {
  return fileFingerprints.get(commitID) ?? ''
}

export type RewriteStackDraftResult<T extends CommitDraft> =
  | { ok: true; commits: T[] }
  | {
    ok: false
    reason:
      | 'empty-stack'
      | 'duplicate-commit-id'
      | 'draft-is-not-a-permutation'
      | 'stack-is-not-visible-head-range'
      | 'draft-crosses-rewrite-range'
      | 'changes-outside-rewrite-range'
  }

export type TargetChainProjectionResult<T extends { commit: string }> =
  | { ok: true; commits: T[] }
  | {
    ok: false
    reason:
      | 'duplicate-commit-id'
      | 'draft-is-not-a-permutation'
      | 'changed-commit-outside-target-chain'
      | 'visible-target-chain-mismatch'
      | 'draft-target-chain-mismatch'
    changedCommitIDs?: string[]
  }

export type DirectionalDropInsertionInput = {
  sourceIndex: number
  targetIndex: number
  pointerY: number
  targetTop: number
  targetHeight: number
  overlapThreshold?: number
}

export type DragPreviewState = {
  insertionIndex: number
  anchorY: number
}

// Reordering the preview can move a different row beneath an otherwise still
// pointer. Keep the active preview for a small movement band so that reflow
// cannot repeatedly clear and recreate the same animation.
export function stabilizeDragPreview(
  previous: DragPreviewState | null,
  candidateInsertionIndex: number | null,
  pointerY: number,
  holdDistance: number,
): DragPreviewState | null {
  if (previous) {
    if (candidateInsertionIndex === previous.insertionIndex) return previous
    if (Math.abs(pointerY - previous.anchorY) < Math.max(0, holdDistance)) return previous
  }

  return candidateInsertionIndex === null
    ? null
    : { insertionIndex: candidateInsertionIndex, anchorY: pointerY }
}

// Native drag events expose the pointer position, not the drag image bounds.
// Treating the pointer's progress through the target row as overlap keeps the
// preview responsive while retaining a direction-aware before/after boundary.
export function directionalDropInsertionIndex({
  sourceIndex,
  targetIndex,
  pointerY,
  targetTop,
  targetHeight,
  overlapThreshold = 0.3,
}: DirectionalDropInsertionInput): number {
  const height = Math.max(1, targetHeight)
  const pointerRatio = Math.max(0, Math.min(1, (pointerY - targetTop) / height))
  const threshold = Math.max(0, Math.min(1, overlapThreshold))

  if (sourceIndex < 0 || sourceIndex === targetIndex) {
    return pointerRatio >= 0.5 ? targetIndex + 1 : targetIndex
  }

  if (sourceIndex < targetIndex) {
    return pointerRatio >= threshold ? targetIndex + 1 : targetIndex
  }

  return 1 - pointerRatio >= threshold ? targetIndex : targetIndex + 1
}

// insertionIndex is measured before the source row is removed. This matches
// native drag/drop targets: 0 is before the first row and length is after it.
export function moveCommitTo<T extends { commit: string }>(commits: T[], commitID: string, insertionIndex: number): T[] {
  const sourceIndex = commits.findIndex((commit) => commit.commit === commitID)
  if (sourceIndex < 0) return commits
  const boundedInsertion = Math.max(0, Math.min(commits.length, insertionIndex))
  const destinationIndex = sourceIndex < boundedInsertion ? boundedInsertion - 1 : boundedInsertion
  if (destinationIndex === sourceIndex) return commits
  const next = [...commits]
  const [moved] = next.splice(sourceIndex, 1)
  next.splice(destinationIndex, 0, moved)
  return next
}

// Only a row the user actively dragged should receive the moved treatment.
// Rows displaced by that action are part of the new draft order, but not the
// visual answer to "which commit did I move?".
export function directlyMovedCommitIDs<T extends { commit: string }>(
  originalCommits: T[],
  draftCommits: T[],
  previousMovedIDs: string[],
  movedCommitID: string,
): string[] {
  const originalIndexes = new Map(originalCommits.map((commit, index) => [commit.commit, index]))
  const moved = previousMovedIDs.filter((commitID) => {
    const originalIndex = originalIndexes.get(commitID)
    return originalIndex !== undefined && draftCommits.findIndex((commit) => commit.commit === commitID) !== originalIndex
  })
  const originalIndex = originalIndexes.get(movedCommitID)
  if (originalIndex !== undefined && draftCommits.findIndex((commit) => commit.commit === movedCommitID) !== originalIndex) moved.push(movedCommitID)
  return [...new Set(moved)]
}

// Content metadata changes are tracked independently from reordering. A commit
// can be back in its original position and still need a replacement commit.
export function commitDraftChangedIDs<T extends {
  commit: string
  message: string
  author: { name: string; email: string }
  date: string
}>(
  originalCommits: T[],
  draftCommits: T[],
  fileFingerprints: CommitFileFingerprints = noFileFingerprints,
): string[] {
  const originalByCommit = new Map(originalCommits.map((commit) => [commit.commit, commit]))
  return draftCommits
    .filter((commit) => {
      const original = originalByCommit.get(commit.commit)
      return !original
        || original.message !== commit.message
        || original.author.name !== commit.author.name
        || original.author.email !== commit.author.email
        || original.date !== commit.date
        || fileFingerprintOf(fileFingerprints, commit.commit) !== ''
    })
    .map((commit) => commit.commit)
}

// Review should name only changes the user made directly. Every later commit
// may receive a replacement hash during replay, but it is not itself a direct
// edit. Keep those two ideas separate for the review surface.
export function commitDraftChangeKinds<T extends CommitDraft>(
  originalCommits: T[],
  draftCommit: T,
  movedCommitIDs: string[],
  fileFingerprints: CommitFileFingerprints = noFileFingerprints,
): CommitDraftChangeKind[] {
  const original = originalCommits.find((commit) => commit.commit === draftCommit.commit)
  if (!original) return []

  const changes: CommitDraftChangeKind[] = []
  if (movedCommitIDs.includes(draftCommit.commit)) changes.push('reordered')
  if (original.message !== draftCommit.message) changes.push('message')
  if (original.author.name !== draftCommit.author.name || original.author.email !== draftCommit.author.email) {
    changes.push('author')
  }
  if (original.date !== draftCommit.date) changes.push('author-date')
  if (fileFingerprintOf(fileFingerprints, draftCommit.commit) !== '') changes.push('files')
  return changes
}

// The fingerprint deliberately contains only data Edit Mode can rewrite today.
// Comparing it after a review invalidates that review for a move, message,
// author, author-date, or file change without reacting to unrelated display
// fields.
export function commitDraftFingerprint<T extends CommitDraft>(
  commits: T[],
  fileFingerprints: CommitFileFingerprints = noFileFingerprints,
): string {
  return JSON.stringify(commits.map((commit) => [
    commit.commit,
    commit.message,
    commit.author.name,
    commit.author.email,
    commit.date,
    fileFingerprintOf(fileFingerprints, commit.commit),
  ]))
}

export function resolveEditTargetBranch(scope: string, allBranches: boolean, defaultBranch: string): string {
  return (allBranches ? defaultBranch : scope).trim()
}

// An All branches draft can contain side rows between target-branch rows.
// Keep only target IDs in draft order, then retain the invisible target tail
// from the original newest-first chain.
export function projectVisualDraftToTargetChain<T extends { commit: string }>(
  originalVisualNewestFirst: T[],
  draftVisualNewestFirst: T[],
  targetChainNewestFirst: T[],
  changedCommitIDs: string[],
): TargetChainProjectionResult<T> {
  const originalIndexes = commitIndexes(originalVisualNewestFirst)
  const draftIndexes = commitIndexes(draftVisualNewestFirst)
  const targetIndexes = commitIndexes(targetChainNewestFirst)
  if (!originalIndexes || !draftIndexes || !targetIndexes) return { ok: false, reason: 'duplicate-commit-id' }
  if (originalVisualNewestFirst.length !== draftVisualNewestFirst.length
    || originalIndexes.size !== draftIndexes.size
    || [...originalIndexes.keys()].some((commitID) => !draftIndexes.has(commitID))) {
    return { ok: false, reason: 'draft-is-not-a-permutation' }
  }

  const targetIDs = new Set(targetIndexes.keys())
  const outsideChangedIDs = [...new Set(changedCommitIDs.filter((commitID) => !targetIDs.has(commitID)))]
  if (outsideChangedIDs.length > 0) {
    return { ok: false, reason: 'changed-commit-outside-target-chain', changedCommitIDs: outsideChangedIDs }
  }

  const originalVisibleTargetIDs = originalVisualNewestFirst
    .filter((commit) => targetIDs.has(commit.commit))
    .map((commit) => commit.commit)
  const expectedVisibleTargetIDs = targetChainNewestFirst
    .slice(0, originalVisibleTargetIDs.length)
    .map((commit) => commit.commit)
  if (originalVisibleTargetIDs.length === 0
    || originalVisibleTargetIDs.length !== expectedVisibleTargetIDs.length
    || originalVisibleTargetIDs.some((commitID, index) => commitID !== expectedVisibleTargetIDs[index])) {
    return { ok: false, reason: 'visible-target-chain-mismatch' }
  }

  const draftVisibleTargetCommits = draftVisualNewestFirst.filter((commit) => targetIDs.has(commit.commit))
  const draftVisibleTargetIDs = draftVisibleTargetCommits.map((commit) => commit.commit)
  const draftVisibleTargetSet = new Set(draftVisibleTargetIDs)
  if (draftVisibleTargetIDs.length !== originalVisibleTargetIDs.length
    || draftVisibleTargetSet.size !== originalVisibleTargetIDs.length
    || originalVisibleTargetIDs.some((commitID) => !draftVisibleTargetSet.has(commitID))) {
    return { ok: false, reason: 'draft-target-chain-mismatch' }
  }

  return {
    ok: true,
    commits: [
      ...draftVisibleTargetCommits,
      ...targetChainNewestFirst.slice(originalVisibleTargetIDs.length),
    ],
  }
}

// Commit rows are displayed newest first. The returned index is therefore the
// oldest directly changed or reordered commit in the original visible list;
// replaying from that row rewrites every newer commit as well. A non-permutation
// is not a valid local rewrite draft, so it has no safe anchor.
export function oldestAffectedOriginalIndex<T extends CommitDraft>(
  originalCommits: T[],
  draftCommits: T[],
  fileFingerprints: CommitFileFingerprints = noFileFingerprints,
): number | null {
  const originalIndexes = commitIndexes(originalCommits)
  const draftIndexes = commitIndexes(draftCommits)
  if (!originalIndexes || !draftIndexes || originalCommits.length !== draftCommits.length) return null
  if (originalIndexes.size !== draftIndexes.size) return null
  if ([...originalIndexes.keys()].some((commitID) => !draftIndexes.has(commitID))) return null

  let oldestAffectedIndex = -1
  for (let draftIndex = 0; draftIndex < draftCommits.length; draftIndex += 1) {
    const draft = draftCommits[draftIndex]
    const originalIndex = originalIndexes.get(draft.commit)
    if (originalIndex === undefined) return null
    const original = originalCommits[originalIndex]
    if (originalIndex !== draftIndex || !sameCommitDraft(original, draft, fileFingerprints)) {
      oldestAffectedIndex = Math.max(oldestAffectedIndex, originalIndex)
    }
  }

  return oldestAffectedIndex >= 0 ? oldestAffectedIndex : null
}

// A backend rewrite stack is oldest first, while the Commit Page is newest
// first. Only use a visual draft when the stack is exactly the visible HEAD
// range and the draft keeps every non-stack row untouched. This prevents a
// filtered or all-branches display from being mistaken for an executable plan.
export function deriveRewriteStackDraft<T extends CommitDraft, S extends { commit: string }>(
  originalVisibleNewestFirst: T[],
  draftVisibleNewestFirst: T[],
  stackOldestFirst: S[],
  fileFingerprints: CommitFileFingerprints = noFileFingerprints,
): RewriteStackDraftResult<T> {
  const originalIndexes = commitIndexes(originalVisibleNewestFirst)
  const draftIndexes = commitIndexes(draftVisibleNewestFirst)
  const stackIndexes = commitIndexes(stackOldestFirst)
  if (!originalIndexes || !draftIndexes || !stackIndexes) return { ok: false, reason: 'duplicate-commit-id' }
  if (stackOldestFirst.length === 0) return { ok: false, reason: 'empty-stack' }
  if (originalVisibleNewestFirst.length !== draftVisibleNewestFirst.length
    || originalIndexes.size !== draftIndexes.size
    || [...originalIndexes.keys()].some((commitID) => !draftIndexes.has(commitID))) {
    return { ok: false, reason: 'draft-is-not-a-permutation' }
  }

  const stackNewestFirst = [...stackOldestFirst].reverse().map((commit) => commit.commit)
  const originalPrefix = originalVisibleNewestFirst.slice(0, stackNewestFirst.length)
  if (originalPrefix.length !== stackNewestFirst.length
    || originalPrefix.some((commit, index) => commit.commit !== stackNewestFirst[index])) {
    return { ok: false, reason: 'stack-is-not-visible-head-range' }
  }

  const draftPrefix = draftVisibleNewestFirst.slice(0, stackNewestFirst.length)
  const draftPrefixIDs = new Set(draftPrefix.map((commit) => commit.commit))
  if (draftPrefix.length !== stackNewestFirst.length
    || draftPrefixIDs.size !== stackNewestFirst.length
    || stackNewestFirst.some((commitID) => !draftPrefixIDs.has(commitID))) {
    return { ok: false, reason: 'draft-crosses-rewrite-range' }
  }

  for (let index = stackNewestFirst.length; index < originalVisibleNewestFirst.length; index += 1) {
    if (!sameCommitDraft(originalVisibleNewestFirst[index], draftVisibleNewestFirst[index], fileFingerprints)) {
      return { ok: false, reason: 'changes-outside-rewrite-range' }
    }
  }

  return { ok: true, commits: [...draftPrefix].reverse() }
}

function commitIndexes<T extends { commit: string }>(commits: T[]): Map<string, number> | null {
  const indexes = new Map<string, number>()
  for (const [index, commit] of commits.entries()) {
    if (indexes.has(commit.commit)) return null
    indexes.set(commit.commit, index)
  }
  return indexes
}

// `left` is always the original commit, which carries no drafted file edits.
// Any file fingerprint recorded for this commit therefore makes the draft
// different even when every metadata field still matches. Without this the
// anchor search would find no change for a file-only edit, and a file edit
// outside the reviewed range would pass the range check and then be dropped
// from the rewrite payload.
function sameCommitDraft<T extends CommitDraft>(
  left: T,
  right: T,
  fileFingerprints: CommitFileFingerprints,
): boolean {
  return left.commit === right.commit
    && left.message === right.message
    && left.author.name === right.author.name
    && left.author.email === right.author.email
    && left.date === right.date
    && fileFingerprintOf(fileFingerprints, right.commit) === ''
}

// Edit Mode starts from the currently visible history. A concrete rewrite
// target is deliberately deferred until the later review/apply phase.
export function commitEditDisabledReason(context: CommitEditContext): string {
  if (!context.hasRepository) return 'Open a repository to edit its visible history.'
  if (context.projectSwitching) return 'Wait for the project switch to finish.'
  if (context.worktreeSwitching) return 'Wait for the worktree switch to finish.'
  return ''
}
