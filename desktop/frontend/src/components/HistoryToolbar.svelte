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
  export let canEditCommits = false
  export let editDisabledReason = ''
  export let onScopeChange: (scope: string, allBranches: boolean) => void
  export let onWorktreeChange: (worktree: WorktreeInfo) => void
  export let onOpenCommitEditor: () => void
  export let onTogglePreset: (id: string) => void

  function changeScope(nextScope: string, nextAllBranches: boolean): void {
    scope = nextScope
    allBranches = nextAllBranches
    onScopeChange(scope, allBranches)
  }

</script>

<section class="history-toolbar" aria-label="Commit controls">
  <div class="history-toolbar-main">
    <h1>Commits</h1>
    <WorktreePicker {worktrees} activeRoot={activeWorktreeRoot} projectRoot={activeProjectRoot} {disabled} onChange={onWorktreeChange} />
    <BranchScopePicker {scope} {allBranches} {branches} {worktrees} {defaultBranch} {currentBranch} {currentDetached} {currentHead} {activeWorktreeRoot} {disabled} onChange={changeScope} />
    <button class="history-action" type="button" on:click={onOpenCommitEditor} disabled={disabled || !canEditCommits} title={editDisabledReason || `Rewrite the selected commit through the checked-out branch${currentBranch ? ` ${currentBranch}` : ''} HEAD`}>Edit commits</button>
  </div>

  <div class="criteria-summary-bar preset-summary-bar" aria-label="Commit filter presets">
    <span class="criteria-summary-label">Preset</span>
    <PresetBar {presets} activeIDs={activePresetIDs} {author} onToggle={onTogglePreset} />
  </div>
</section>
