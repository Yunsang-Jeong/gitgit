<script lang="ts">
  import BranchScopePicker from './BranchScopePicker.svelte'
  import PresetBar from './PresetBar.svelte'
  import WorktreePicker from './WorktreePicker.svelte'
  import type { Author, CommitFilterAction, CommitFilterField, CommitFilterJoin, CommitFilterLogic, CommitFilterPreset, CommitFilterRule, WorktreeInfo } from '../lib/types'

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
  export let rules: CommitFilterRule[] = []
  export let presets: CommitFilterPreset[] = []
  export let activePresetIDs: string[] = []
  export let author: Author = { name: '', email: '' }
  export let disabled = false
  export let canEditCommits = false
  export let editDisabledReason = ''
  export let logic: CommitFilterLogic
  export let onScopeChange: (scope: string, allBranches: boolean) => void
  export let onWorktreeChange: (worktree: WorktreeInfo) => void
  export let onOpenCommitEditor: () => void
  export let onRulesChange: (rules: CommitFilterRule[]) => void
  export let onLogicChange: (logic: CommitFilterLogic) => void
  export let onTogglePreset: (id: string) => void

  let editorOpen = false
  let draftAction: CommitFilterAction = 'highlight'
  let draftField: CommitFilterField = 'branch'
  let draftPattern = ''

  function changeScope(nextScope: string, nextAllBranches: boolean): void {
    scope = nextScope
    allBranches = nextAllBranches
    onScopeChange(scope, allBranches)
  }

  function addRule(): void {
    const pattern = draftPattern.trim()
    if (!pattern) return
    rules = [...rules, { id: `${Date.now()}-${Math.random()}`, action: draftAction, field: draftField, pattern }]
    onRulesChange(rules)
    draftPattern = ''
    editorOpen = false
  }

  function removeRule(id: string): void {
    rules = rules.filter((rule) => rule.id !== id)
    onRulesChange(rules)
  }

  function changeLogic(action: 'show' | 'hide', join: CommitFilterJoin): void {
    logic = { ...logic, [action]: join }
    onLogicChange(logic)
  }

  function handleEditorKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault()
      addRule()
    } else if (event.key === 'Escape') {
      editorOpen = false
    }
  }
</script>

<section class="history-toolbar" aria-label="Commit controls">
  <div class="history-toolbar-main">
    <h1>Commits</h1>
    <WorktreePicker {worktrees} activeRoot={activeWorktreeRoot} projectRoot={activeProjectRoot} {disabled} onChange={onWorktreeChange} />
    <BranchScopePicker {scope} {allBranches} {branches} {worktrees} {defaultBranch} {currentBranch} {currentDetached} {currentHead} {activeWorktreeRoot} {disabled} onChange={changeScope} />
    <button class="history-action" type="button" on:click={onOpenCommitEditor} disabled={disabled || !canEditCommits} title={editDisabledReason || `Rewrite the selected commit through the checked-out branch${currentBranch ? ` ${currentBranch}` : ''} HEAD`}>Edit commits</button>
    <span class="toolbar-spacer"></span>
    <button class:active={editorOpen} class="history-action" type="button" on:click={() => (editorOpen = !editorOpen)} disabled={disabled}>＋ Filter</button>
  </div>

  <div class="criteria-summary-bar preset-summary-bar" aria-label="Commit filter presets">
    <span class="criteria-summary-label">Preset</span>
    <PresetBar {presets} activeIDs={activePresetIDs} {author} onToggle={onTogglePreset} />
  </div>

  {#if editorOpen}
    <div class="filter-drawer">
      <div class="filter-drawer-title">
        <strong>Filter commits</strong>
        <span>Branch, author, message, changed file, and date</span>
        <button type="button" on:click={() => (editorOpen = false)} aria-label="Close filter">×</button>
      </div>
      <div class="filter-composer">
        <div class="filter-editor">
          <select bind:value={draftAction} aria-label="Filter action">
            <option value="highlight">Highlight</option>
            <option value="hide">Hide</option>
            <option value="show">Show</option>
          </select>
          <select bind:value={draftField} aria-label="Filter field">
            <option value="branch">Branch</option>
            <option value="author">Author</option>
            <option value="message">Message</option>
            <option value="file">Changed file</option>
            <option value="date">Date</option>
          </select>
          <input bind:value={draftPattern} on:keydown={handleEditorKeydown} placeholder={draftField === 'file' ? 'internal/**' : draftField === 'branch' ? 'feature/*' : draftField === 'date' ? 'last:3d or 2026. 7. 19.' : 'pattern'} aria-label="Filter pattern" />
          <button type="button" on:click={addRule} disabled={!draftPattern.trim()}>Add rule</button>
          <div class="filter-logic-controls" aria-label="Filter combination logic">
            <label>Show
              <select value={logic.show} on:change={(event) => changeLogic('show', (event.currentTarget as HTMLSelectElement).value as CommitFilterJoin)} aria-label="Combine Show filters">
                <option value="and">AND</option><option value="or">OR</option>
              </select>
            </label>
            <label>Hide
              <select value={logic.hide} on:change={(event) => changeLogic('hide', (event.currentTarget as HTMLSelectElement).value as CommitFilterJoin)} aria-label="Combine Hide filters">
                <option value="and">AND</option><option value="or">OR</option>
              </select>
            </label>
          </div>
        </div>
      </div>
    </div>
  {/if}

  {#if rules.length > 0}
    <div class="criteria-summary-bar filter-rules" aria-label="Active commit filters">
      <span class="criteria-summary-label">Filters</span>
      <div class="criteria-summary-items filter-rule-list">
        {#each rules as rule}
          <span class="filter-rule action-{rule.action}">
            <strong>{rule.action}</strong>
            <span>{rule.field}: <code>{rule.pattern}</code></span>
            <button type="button" on:click={() => removeRule(rule.id)} aria-label="Remove {rule.action} {rule.field} filter">×</button>
          </span>
        {/each}
      </div>
    </div>
  {/if}
</section>
