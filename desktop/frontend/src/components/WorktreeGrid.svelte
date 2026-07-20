<script lang="ts">
  import { onMount, tick } from 'svelte'
  import WorktreeCard from './WorktreeCard.svelte'
  import type { RepositoryState, WorktreeInfo } from '../lib/types'

  export let repository: RepositoryState
  export let activeProjectRoot = ''
  export let removing = false
  export let onView: (worktree: WorktreeInfo) => void
  export let onOpen: (worktree: WorktreeInfo) => void
  export let onOpenIDE: (worktree: WorktreeInfo) => void
  export let onRemove: (worktrees: WorktreeInfo[]) => Promise<boolean>

  let selectedPaths: string[] = []
  let selectionAnchor = ''
  let actionsOpen = false
  let pendingRemoval: WorktreeInfo[] = []
  let confirmingRemoval = false
  let confirmButton: HTMLButtonElement
  let actionsRoot: HTMLDivElement

  $: orderedWorktrees = sortWorktrees(repository.worktrees, activeProjectRoot)
  $: mainWorktrees = orderedWorktrees.filter((worktree) => worktree.path === activeProjectRoot)
  $: mergedWorktrees = orderedWorktrees.filter((worktree) => worktree.path !== activeProjectRoot && worktree.merged_into_default)
  $: unmergedWorktrees = orderedWorktrees.filter((worktree) => worktree.path !== activeProjectRoot && !worktree.merged_into_default)
  $: selectablePaths = new Set(orderedWorktrees.filter((worktree) => worktree.path !== activeProjectRoot).map((worktree) => worktree.path))
  $: selectedWorktrees = orderedWorktrees.filter((worktree) => selectedPaths.includes(worktree.path))
  $: removableWorktrees = orderedWorktrees.filter((worktree) => removalBlocker(worktree) === '')
  $: removalReason = selectedRemovalBlocker(selectedWorktrees)
  $: if (selectedPaths.some((path) => !selectablePaths.has(path))) {
    selectedPaths = selectedPaths.filter((path) => selectablePaths.has(path))
  }

  onMount(() => {
    const closeActions = (event: PointerEvent) => {
      if (actionsOpen && event.target instanceof Node && !actionsRoot?.contains(event.target)) actionsOpen = false
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      actionsOpen = false
      if (!confirmingRemoval) pendingRemoval = []
    }
    window.addEventListener('pointerdown', closeActions)
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      window.removeEventListener('pointerdown', closeActions)
      window.removeEventListener('keydown', closeOnEscape)
    }
  })

  function sortWorktrees(worktrees: WorktreeInfo[], mainPath: string): WorktreeInfo[] {
    return [...worktrees].sort((left, right) => compareWorktrees(left, right, mainPath))
  }

  function compareWorktrees(left: WorktreeInfo, right: WorktreeInfo, mainPath: string): number {
    const leftMain = left.path === mainPath
    const rightMain = right.path === mainPath
    if (leftMain !== rightMain) return leftMain ? -1 : 1
    const leftName = left.detached ? `detached/${left.head}` : left.branch || left.path
    const rightName = right.detached ? `detached/${right.head}` : right.branch || right.path
    return leftName.localeCompare(rightName, undefined, { numeric: true, sensitivity: 'base' }) || left.path.localeCompare(right.path)
  }

  function removalBlocker(worktree: WorktreeInfo): string {
    if (worktree.path === activeProjectRoot) return 'Main worktree'
    if (worktree.path === repository.root) return 'Switch to Main first'
    if (worktree.detached || !worktree.branch) return 'Detached worktree'
    if (worktree.branch === repository.default_branch) return 'Default branch'
    if (worktree.locked) return 'Locked worktree'
    if (worktree.dirty) return 'Local changes present'
    if (!worktree.merged_into_default) return `Not merged into ${repository.default_branch}`
    return ''
  }

  function selectedRemovalBlocker(worktrees: WorktreeInfo[]): string {
    if (worktrees.length === 0) return 'Select one or more worktrees'
    for (const worktree of worktrees) {
      const reason = removalBlocker(worktree)
      if (reason) return `${worktree.branch || 'detached'}: ${reason}`
    }
    return ''
  }

  function setWorktreeSelected(worktree: WorktreeInfo, checked: boolean, range = false): void {
    if (worktree.path === activeProjectRoot) return
    const paths = new Set(selectedPaths)
    if (range && selectionAnchor) {
      const anchorIndex = orderedWorktrees.findIndex((candidate) => candidate.path === selectionAnchor)
      const targetIndex = orderedWorktrees.findIndex((candidate) => candidate.path === worktree.path)
      if (anchorIndex >= 0 && targetIndex >= 0) {
        const start = Math.min(anchorIndex, targetIndex)
        const end = Math.max(anchorIndex, targetIndex)
        for (const candidate of orderedWorktrees.slice(start, end + 1)) {
          if (candidate.path === activeProjectRoot) continue
          if (checked) paths.add(candidate.path)
          else paths.delete(candidate.path)
        }
      }
    } else if (checked) {
      paths.add(worktree.path)
    } else {
      paths.delete(worktree.path)
    }
    selectedPaths = [...paths]
    selectionAnchor = worktree.path
    actionsOpen = false
  }

  function clearSelection(): void {
    selectedPaths = []
    selectionAnchor = ''
    actionsOpen = false
  }

  function runSingleAction(action: (worktree: WorktreeInfo) => void): void {
    if (selectedWorktrees.length !== 1) return
    actionsOpen = false
    action(selectedWorktrees[0])
  }

  async function requestRemoval(worktrees: WorktreeInfo[]): Promise<void> {
    if (worktrees.length === 0 || worktrees.some((worktree) => removalBlocker(worktree))) return
    actionsOpen = false
    pendingRemoval = [...worktrees]
    await tick()
    confirmButton?.focus()
  }

  async function confirmRemoval(): Promise<void> {
    if (pendingRemoval.length === 0 || confirmingRemoval) return
    const worktrees = [...pendingRemoval]
    confirmingRemoval = true
    try {
      if (await onRemove(worktrees)) {
        clearSelection()
        pendingRemoval = []
      }
    } finally {
      confirmingRemoval = false
    }
  }
</script>

<section class="worktree-workspace pane">
  <header class="workspace-view-header">
    <div>
      <h1>Worktrees</h1>
      <p>{repository.worktrees.length} {repository.worktrees.length === 1 ? 'worktree' : 'worktrees'} · default branch <strong>{repository.default_branch}</strong></p>
    </div>

    <div class="worktree-header-actions">
      {#if selectedWorktrees.length > 0}
        <span class="worktree-selection-count">{selectedWorktrees.length} selected</span>
        <button class="worktree-clear-selection" type="button" on:click={clearSelection}>Clear</button>
      {/if}
      <div bind:this={actionsRoot} class="worktree-bulk-actions">
        <button
          class:open={actionsOpen}
          class="worktree-actions-trigger"
          type="button"
          aria-haspopup="menu"
          aria-expanded={actionsOpen}
          disabled={selectedWorktrees.length === 0 || removing}
          on:click={() => (actionsOpen = !actionsOpen)}
        >Actions <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg></button>
        {#if actionsOpen}
          <div class="worktree-actions-menu" role="menu">
            <button type="button" role="menuitem" disabled={selectedWorktrees.length !== 1} on:click={() => runSingleAction(onView)}>
              View commits
              {#if selectedWorktrees.length !== 1}<small>Select exactly one worktree</small>{/if}
            </button>
            <button type="button" role="menuitem" disabled={selectedWorktrees.length !== 1} on:click={() => runSingleAction(onOpen)}>
              Open in Finder
              {#if selectedWorktrees.length !== 1}<small>Select exactly one worktree</small>{/if}
            </button>
            <button class="danger" type="button" role="menuitem" disabled={Boolean(removalReason)} on:click={() => void requestRemoval(selectedWorktrees)}>
              Remove {selectedWorktrees.length} worktree{selectedWorktrees.length === 1 ? '' : 's'} &amp; branches
              {#if removalReason}<small>{removalReason}</small>{/if}
            </button>
          </div>
        {/if}
      </div>
      <button
        class="worktree-clear-merged"
        type="button"
        disabled={removableWorktrees.length === 0 || removing}
        on:click={() => void requestRemoval(removableWorktrees)}
      >Clear merged worktrees</button>
    </div>
  </header>

  <div class="worktree-groups" aria-label="Worktrees grouped by merge state">
    <section class="worktree-group">
      <header><h2>Main</h2><span>Primary checkout</span></header>
      <div class="worktree-group-grid">
        {#each mainWorktrees as worktree (worktree.path)}
          <WorktreeCard
            {worktree}
            defaultBranch={repository.default_branch}
            main
            selected={false}
            onToggle={setWorktreeSelected}
            {onOpenIDE}
          />
        {/each}
      </div>
    </section>

    <section class="worktree-group">
      <header><h2>Merged</h2><span>{removableWorktrees.length} ready to clean</span></header>
      {#if mergedWorktrees.length > 0}
        <div class="worktree-group-grid">
          {#each mergedWorktrees as worktree (worktree.path)}
            <WorktreeCard
              {worktree}
              defaultBranch={repository.default_branch}
              selected={selectedPaths.includes(worktree.path)}
              onToggle={setWorktreeSelected}
              {onOpenIDE}
            />
          {/each}
        </div>
      {:else}
        <p class="worktree-group-empty">No merged worktrees.</p>
      {/if}
    </section>

    <section class="worktree-group">
      <header><h2>Unmerged</h2><span>Active or protected work</span></header>
      {#if unmergedWorktrees.length > 0}
        <div class="worktree-group-grid">
          {#each unmergedWorktrees as worktree (worktree.path)}
            <WorktreeCard
              {worktree}
              defaultBranch={repository.default_branch}
              selected={selectedPaths.includes(worktree.path)}
              onToggle={setWorktreeSelected}
              {onOpenIDE}
            />
          {/each}
        </div>
      {:else}
        <p class="worktree-group-empty">No unmerged worktrees.</p>
      {/if}
    </section>
  </div>

  {#if pendingRemoval.length > 0}
    <div class="worktree-confirm-backdrop" role="presentation" on:mousedown={() => { if (!confirmingRemoval) pendingRemoval = [] }}>
      <div class="worktree-confirm" role="alertdialog" aria-modal="true" aria-labelledby="remove-worktree-title" tabindex="-1" on:mousedown|stopPropagation>
        <h2 id="remove-worktree-title">Remove {pendingRemoval.length} merged worktree{pendingRemoval.length === 1 ? '' : 's'}?</h2>
        <p>The selected worktree directories and their local branches will be removed. GitGit validates every selection before making changes.</p>
        <div class="worktree-confirm-list">
          {#each pendingRemoval as worktree (worktree.path)}
            <div>
              <strong>{worktree.branch}</strong>
              <code title={worktree.path}>{worktree.path}</code>
            </div>
          {/each}
        </div>
        <div class="worktree-confirm-actions">
          <button type="button" disabled={confirmingRemoval} on:click={() => (pendingRemoval = [])}>Cancel</button>
          <button bind:this={confirmButton} class="danger" type="button" disabled={confirmingRemoval} on:click={() => void confirmRemoval()}>{confirmingRemoval ? 'Removing…' : 'Remove selected'}</button>
        </div>
      </div>
    </div>
  {/if}
</section>
