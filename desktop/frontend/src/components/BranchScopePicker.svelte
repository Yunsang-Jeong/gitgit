<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { matchingBranches, nextBranchChoice, nextVisibleBranchCount, orderedBranches, type BranchNavigationKey } from '../lib/branch-options'
  import type { WorktreeInfo } from '../lib/types'

  export let scope = 'HEAD'
  export let allBranches = false
  export let branches: string[] = []
  export let worktrees: WorktreeInfo[] = []
  export let defaultBranch = ''
  export let currentBranch = ''
  export let currentDetached = false
  export let currentHead = ''
  export let activeWorktreeRoot = ''
  export let disabled = false
  export let onChange: (scope: string, allBranches: boolean) => void

  const pageSize = 25
  const listboxID = 'commit-branch-options'
  const allChoice = '\0all-branches'
  const detachedChoice = '\0detached-head'
  let open = false
  let query = ''
  let visibleCount = pageSize
  let activeScope = ''
  let pinnedScope = ''
  let keyboardScope = ''
  let root: HTMLDivElement
  let trigger: HTMLButtonElement
  let searchInput: HTMLInputElement

  $: worktreeByBranch = new Map(worktrees.filter((worktree) => worktree.branch).map((worktree) => [worktree.branch as string, worktree]))
  $: orderedOptions = orderedBranches(branches, defaultBranch, currentBranch)
  $: filteredOptions = matchingBranches(orderedOptions, query)
  $: selectedScope = scope
  $: selectedLabel = allBranches
    ? 'All branches'
    : selectedScope === 'HEAD' && currentDetached
      ? `Detached · ${currentHead.slice(0, 8) || 'HEAD'}`
      : selectedScope
  $: normalizedQuery = query.trim().toLocaleLowerCase()
  $: showAll = !normalizedQuery || 'all branches'.includes(normalizedQuery)
  $: detachedHeadLabel = `Detached HEAD ${currentHead.slice(0, 8) || ''}`.trim()
  $: showDetachedHead = currentDetached && (!normalizedQuery || detachedHeadLabel.toLocaleLowerCase().includes(normalizedQuery))
  $: pinnedOption = !normalizedQuery && pinnedScope && filteredOptions.includes(pinnedScope) ? pinnedScope : ''
  $: visiblePageOptions = filteredOptions.slice(0, visibleCount)
  $: keyboardOption = keyboardScope
    && keyboardScope !== pinnedOption
    && filteredOptions.includes(keyboardScope)
    && !visiblePageOptions.includes(keyboardScope)
    ? keyboardScope
    : ''
  $: visibleOptions = visiblePageOptions.filter((branch) => branch !== pinnedOption && branch !== keyboardOption)
  $: renderedOptions = [...(pinnedOption ? [pinnedOption] : []), ...visibleOptions, ...(keyboardOption ? [keyboardOption] : [])]
  $: optionScopes = [...(showAll ? [allChoice] : []), ...(showDetachedHead ? [detachedChoice] : []), ...renderedOptions]
  $: activeDescendant = activeScope && optionScopes.includes(activeScope) ? optionID(activeScope) : undefined

  onMount(() => {
    const closeOutside = (event: PointerEvent) => {
      if (open && event.target instanceof Node && !root?.contains(event.target)) close(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && open) close(true)
    }
    window.addEventListener('pointerdown', closeOutside)
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      window.removeEventListener('pointerdown', closeOutside)
      window.removeEventListener('keydown', closeOnEscape)
    }
  })

  async function toggle(): Promise<void> {
    open = !open
    if (!open) {
      resetMenu()
      return
    }
    visibleCount = pageSize
    pinnedScope = !allBranches
      && filteredOptions.includes(selectedScope)
      && !filteredOptions.slice(0, pageSize).includes(selectedScope)
      ? selectedScope
      : ''
    activeScope = allBranches ? allChoice : currentDetached && selectedScope === 'HEAD' ? detachedChoice : selectedScope
    await tick()
    searchInput?.focus()
  }

  function close(restoreFocus: boolean): void {
    open = false
    resetMenu()
    if (restoreFocus) void tick().then(() => trigger?.focus())
  }

  function resetMenu(): void {
    query = ''
    visibleCount = pageSize
    activeScope = ''
    pinnedScope = ''
    keyboardScope = ''
  }

  function resetQuery(event: Event): void {
    const nextQuery = (event.currentTarget as HTMLInputElement).value
    const nextPattern = nextQuery.trim().toLocaleLowerCase()
    const nextMatches = matchingBranches(orderedOptions, nextQuery)
    visibleCount = pageSize
    keyboardScope = ''
    activeScope = !nextPattern || 'all branches'.includes(nextPattern)
      ? allChoice
      : currentDetached && detachedHeadLabel.toLocaleLowerCase().includes(nextPattern)
        ? detachedChoice
        : nextMatches[0] ?? ''
  }

  function select(nextScope: string, nextAllBranches = false): void {
    scope = nextScope
    allBranches = nextAllBranches
    close(true)
    onChange(nextScope, nextAllBranches)
  }

  function handleScroll(event: Event): void {
    if (visibleCount >= filteredOptions.length) return
    const target = event.currentTarget as HTMLDivElement
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight
    if (remaining <= target.clientHeight / 2) visibleCount = nextVisibleBranchCount(visibleCount, filteredOptions.length, pageSize)
  }

  async function handleSearchKeydown(event: KeyboardEvent): Promise<void> {
    if (event.key === 'Tab') {
      close(false)
      return
    }
    if (event.isComposing || !['ArrowDown', 'ArrowUp', 'Home', 'End', 'PageDown', 'PageUp', 'Enter', 'Escape'].includes(event.key)) return
    event.preventDefault()
    if (event.key === 'Escape') {
      close(true)
      return
    }
    if (event.key === 'Enter') {
      if (activeScope === allChoice) select(scope, true)
      else if (activeScope === detachedChoice) select('HEAD')
      else if (activeScope) select(activeScope)
      return
    }
    const choices = [...(showAll ? [allChoice] : []), ...(showDetachedHead ? [detachedChoice] : []), ...filteredOptions]
    if (choices.length === 0) return
    const nextScope = nextBranchChoice(choices, activeScope, event.key as BranchNavigationKey)
    const nextBranchIndex = filteredOptions.indexOf(nextScope)
    const canRevealNextPage = (event.key === 'ArrowDown' || event.key === 'PageDown')
      && nextBranchIndex >= visibleCount
      && nextBranchIndex < visibleCount + pageSize
    if (canRevealNextPage) {
      visibleCount = nextVisibleBranchCount(visibleCount, filteredOptions.length, pageSize)
      keyboardScope = ''
    } else if (nextBranchIndex >= visibleCount && nextScope !== pinnedOption) {
      keyboardScope = nextScope
    } else {
      keyboardScope = ''
    }
    activeScope = nextScope
    await tick()
    root?.querySelector<HTMLElement>(`#${CSS.escape(optionID(activeScope))}`)?.scrollIntoView({ block: 'nearest' })
  }

  function optionID(value: string): string {
    if (value === allChoice) return 'branch-option-all'
    if (value === detachedChoice) return 'branch-option-detached'
    return `branch-option-${encodeURIComponent(value).replaceAll('%', '-')}`
  }
</script>

<div class="branch-scope-picker" bind:this={root}>
  <button
    bind:this={trigger}
    class:open
    class="scope-picker-trigger branch-scope-trigger"
    type="button"
    {disabled}
    aria-label={`Branch scope: ${selectedLabel}`}
    aria-haspopup="dialog"
    aria-expanded={open}
    on:click={() => void toggle()}
  >
    <span class="scope-trigger-copy"><small>Branch</small><strong>{selectedLabel}</strong></span>
    {#if !allBranches && ((selectedScope === currentBranch && currentBranch && !currentDetached) || (selectedScope === 'HEAD' && currentDetached))}<b class="scope-badge current">Current</b>{/if}
    <svg class="scope-picker-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
  </button>

  {#if open}
    <div class="branch-scope-menu" role="dialog" aria-label="Select branch scope">
      <div class="scope-picker-search">
        <span>⌕</span>
        <input
          bind:this={searchInput}
          bind:value={query}
          type="search"
          role="combobox"
          placeholder="Search branches…"
          aria-label="Search local branches"
          aria-autocomplete="list"
          aria-controls={listboxID}
          aria-expanded="true"
          aria-activedescendant={activeDescendant}
          on:input={resetQuery}
          on:keydown={(event) => void handleSearchKeydown(event)}
        />
      </div>
      <div id={listboxID} class="branch-scope-list" role="listbox" aria-label="Local branches" on:scroll={handleScroll}>
        {#if showAll}
          <button id={optionID(allChoice)} class:selected={allBranches} class:keyboard-active={activeScope === allChoice} type="button" role="option" aria-selected={allBranches} tabindex="-1" on:mouseenter={() => (activeScope = allChoice)} on:click={() => select(scope, true)}>
            <span class="scope-option-copy"><strong>All branches</strong><small>Local branches + matching remote default branches</small></span>
          </button>
        {/if}
        {#if showDetachedHead}
          <button id={optionID(detachedChoice)} class:selected={!allBranches && selectedScope === 'HEAD'} class:keyboard-active={activeScope === detachedChoice} type="button" role="option" aria-selected={!allBranches && selectedScope === 'HEAD'} tabindex="-1" on:mouseenter={() => (activeScope = detachedChoice)} on:click={() => select('HEAD')}>
            <span class="scope-option-copy"><strong>Detached HEAD</strong><small>{currentHead.slice(0, 8) || 'unborn'}</small></span>
            <span class="scope-option-badges"><b class="scope-badge current">Current</b></span>
          </button>
        {/if}
        {#each renderedOptions as branch (branch)}
          {@const worktree = worktreeByBranch.get(branch)}
          <button id={optionID(branch)} class:selected={!allBranches && selectedScope === branch} class:keyboard-active={activeScope === branch} type="button" role="option" aria-selected={!allBranches && selectedScope === branch} tabindex="-1" on:mouseenter={() => (activeScope = branch)} on:click={() => select(branch)}>
            <span class="scope-option-copy">
              <strong>{branch}</strong>
              {#if worktree && worktree.path !== activeWorktreeRoot}<small title={worktree.path}>{worktree.path}</small>{/if}
            </span>
            <span class="scope-option-badges">
              {#if branch === currentBranch && !currentDetached}<b class="scope-badge current">Current</b>{/if}
              {#if branch === defaultBranch}<b class="scope-badge default">Default</b>{/if}
              {#if worktree && worktree.path !== activeWorktreeRoot}<b class="scope-badge">Other worktree</b>{/if}
              {#if !worktree}<b class="scope-badge muted">No worktree</b>{/if}
            </span>
          </button>
        {/each}
      </div>
      {#if filteredOptions.length === 0 && !showAll}
        <p class="scope-picker-empty" role="status" aria-live="polite">No branches match “{query.trim()}”</p>
      {:else if visibleCount < filteredOptions.length}
        <p class="scope-picker-status" role="status" aria-live="polite">{Math.min(visibleCount, filteredOptions.length)} of {filteredOptions.length} branches</p>
      {/if}
    </div>
  {/if}
</div>
