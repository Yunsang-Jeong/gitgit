<script lang="ts">
  import BranchScopePicker from './BranchScopePicker.svelte'
  import PresetBar from './PresetBar.svelte'
  import WorktreePicker from './WorktreePicker.svelte'
  import type { Author, CommitFilterPreset, WorktreeInfo } from '../lib/types'

  export let scope = 'HEAD'
  export let allBranches = false
  export let branches: string[] = []
  export let worktrees: WorktreeInfo[] = []
  export let defaultBranch = ''
  export let currentBranch = ''
  export let currentDetached = false
  export let currentHead = ''
  export let activeProjectRoot = ''
  export let activeWorktreeRoot = ''
  export let presets: CommitFilterPreset[] = []
  export let activePresetIDs: string[] = []
  export let author: Author = { name: '', email: '' }
  export let disabled = false
  export let editMode = false
  export let canEditCommits = false
  export let editDisabledReason = ''
  export let editModeActionDisabled = false
  export let worktreeActionsDisabled = false
  export let onScopeChange: (scope: string, allBranches: boolean) => void
  export let onWorktreeChange: (worktree: WorktreeInfo) => void
  export let onTogglePreset: (id: string) => void
  export let onEnterEditMode: () => void = () => undefined
  export let onExitEditMode: () => void = () => undefined
  export let onOpenCurrentWorktree: () => void = () => undefined
  export let onOpenCurrentWorktreeInTerminal: () => void = () => undefined
  export let onOpenCurrentWorktreeInIDE: () => void = () => undefined

  function changeScope(nextScope: string, nextAllBranches: boolean): void {
    scope = nextScope
    allBranches = nextAllBranches
    onScopeChange(scope, allBranches)
  }

</script>

<section class="history-toolbar" aria-label="Commit controls">
  <div class="history-toolbar-main">
    <WorktreePicker {worktrees} activeRoot={activeWorktreeRoot} projectRoot={activeProjectRoot} disabled={disabled || editMode} onChange={onWorktreeChange} />
    <BranchScopePicker {scope} {allBranches} {branches} {worktrees} {defaultBranch} {currentBranch} {currentDetached} {currentHead} {activeWorktreeRoot} disabled={disabled || editMode} onChange={changeScope} />

    <div class="history-toolbar-quick-actions">
      <PresetBar {presets} activeIDs={activePresetIDs} {author} disabled={disabled || editMode} onToggle={onTogglePreset} />

      <div class="history-worktree-actions" aria-label="Current worktree actions">
        <button
          class:edit-mode-active={editMode}
          class="history-worktree-action history-edit-mode-action"
          type="button"
          aria-pressed={editMode}
          on:click={editMode ? onExitEditMode : onEnterEditMode}
          disabled={editModeActionDisabled || (!editMode && !canEditCommits)}
          title={editMode ? 'Exit Edit Mode and discard this local reorder draft' : editDisabledReason || 'Enter Edit Mode for the visible history'}
        ><span aria-hidden="true">✎</span><span>{editMode ? 'Exit Edit' : 'Edit Mode'}</span></button>
        <button class="history-worktree-action" type="button" on:click={onOpenCurrentWorktree} disabled={worktreeActionsDisabled} title="Open the current worktree in Finder" aria-label="Open current worktree in Finder"><span aria-hidden="true">▱</span><span>Finder</span></button>
        <button class="history-worktree-action" type="button" on:click={onOpenCurrentWorktreeInTerminal} disabled={worktreeActionsDisabled} title="Open the current worktree in Terminal" aria-label="Open current worktree in Terminal"><span aria-hidden="true">⌘</span><span>Terminal</span></button>
        <button class="history-worktree-action" type="button" on:click={onOpenCurrentWorktreeInIDE} disabled={worktreeActionsDisabled} title="Open the current worktree in the configured IDE" aria-label="Open current worktree in IDE"><span aria-hidden="true">↗</span><span>IDE</span></button>
      </div>
    </div>
  </div>
</section>
