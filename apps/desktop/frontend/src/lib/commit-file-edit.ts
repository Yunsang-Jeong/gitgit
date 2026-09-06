import type { CommitFileContent, CommitFileDraft, CommitFileEdit } from './types'

// A draft keeps the commit's current file state in the inherited
// `content`/`exists` fields and the untouched original alongside it. `exists`
// is therefore the drafted state: false means the rewritten commit deletes the
// path, which is also how a commit that already deletes it starts out.
export function createFileDraft(content: CommitFileContent): CommitFileDraft {
  return {
    ...content,
    original_content: content.content,
    original_delete: !content.exists,
    dirty: false,
  }
}

export function fileDraftDeletes(draft: CommitFileDraft): boolean {
  return !draft.exists
}

export function updateFileDraftContent(draft: CommitFileDraft, content: string): CommitFileDraft {
  return settleFileDraft({ ...draft, content, exists: true })
}

export function markFileDraftDeleted(draft: CommitFileDraft): CommitFileDraft {
  return settleFileDraft({ ...draft, content: '', exists: false })
}

// Restoring only makes sense for a path the commit deletes; the backend fills
// `restore_content` from the first parent so the rewritten commit can keep it.
export function restoreFileDraft(draft: CommitFileDraft): CommitFileDraft {
  if (!draft.restorable) return draft
  return settleFileDraft({ ...draft, content: draft.restore_content ?? '', exists: true })
}

export function revertFileDraft(draft: CommitFileDraft): CommitFileDraft {
  return settleFileDraft({
    ...draft,
    content: draft.original_content,
    exists: !draft.original_delete,
  })
}

// A draft that has come back to its original content and delete state is not a
// change, so it must not reach the rewrite payload or invalidate a review.
function settleFileDraft(draft: CommitFileDraft): CommitFileDraft {
  const dirty = draft.content !== draft.original_content || !draft.exists !== draft.original_delete
  return { ...draft, dirty }
}

export function fileDraftsToEdits(drafts: Iterable<CommitFileDraft>): CommitFileEdit[] {
  return [...drafts]
    .filter((draft) => draft.dirty)
    .map((draft) => ({
      path: draft.path,
      content: draft.exists ? draft.content : '',
      delete: !draft.exists,
    }))
    .sort((left, right) => (left.path < right.path ? -1 : left.path > right.path ? 1 : 0))
}

// Reviews are invalidated by comparing draft fingerprints. File state has to be
// part of that comparison, otherwise an edited file would keep a stale review
// valid and could be applied without ever being verified.
export function fileDraftFingerprint(drafts: Iterable<CommitFileDraft>): string {
  const edits = fileDraftsToEdits(drafts)
  if (edits.length === 0) return ''
  return JSON.stringify(edits.map((edit) => [edit.path, edit.delete, edit.content]))
}
