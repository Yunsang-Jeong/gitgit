<script lang="ts">
  import { onMount, tick } from 'svelte'
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { matchingBranches, nextBranchChoice, nextVisibleBranchCount, orderedBranches, type BranchNavigationKey } from '../lib/branch-options'
  import { isRemoteBranchScope, remoteBranchLabel, remoteScopeUnavailable, visibleRemoteBranches } from '../lib/remote-branches'
  import { resolveRefBadge } from '../lib/remotes'
  import type { RemoteBadgeRule, RemoteBranchCatalogEntry, RemoteInfo, WorktreeInfo } from '../lib/types'

  export let scope = 'HEAD'
  export let allBranches = false
  export let branches: string[] = []
  export let remoteCatalog: RemoteBranchCatalogEntry[] = []
  export let remotes: RemoteInfo[] = []
  export let remoteBadgeRules: RemoteBadgeRule[] = []
  export let worktrees: WorktreeInfo[] = []
  export let defaultBranch = ''
  export let currentBranch = ''
  export let currentDetached = false
  export let currentHead = ''
  export let activeWorktreeRoot = ''
  export let allLabel = 'All branches'
  export let allDescription = 'Local branches + matching remote default branches'
  export let disabled = false
  export let onChange: (scope: string, allBranches: boolean) => void
  export let onOpen: () => void = () => undefined
  export let onRetryRemote: (remote: string) => void = () => undefined

  const pageSize = 25
  const listboxID = 'commit-branch-options'
  const allChoice = '\0all-branches'
  const detachedChoice = '\0detached-head'
  let open = false
  let query = ''
  let visibleCount = pageSize
  let visibleRemoteCount = pageSize
  let activeScope = ''
  let pinnedScope = ''
  let keyboardScope = ''
  let root: HTMLDivElement
  let trigger: HTMLButtonElement
  let searchInput: HTMLInputElement

  $: worktreeByBranch = new Map(worktrees.filter((worktree) => worktree.branch).map((worktree) => [worktree.branch as string, worktree]))
  $: orderedOptions = orderedBranches(branches, defaultBranch, currentBranch)
  $: filteredOptions = matchingBranches(orderedOptions, query)
  $: remoteBranches = visibleRemoteBranches(remoteCatalog)
  $: remoteBranchByRef = new Map(remoteBranches.map((branch) => [branch.ref, branch]))
  $: orderedRemoteScopes = [...remoteBranches]
    .sort((left, right) => Number(right.default) - Number(left.default) || remoteBranchLabel(left.ref).localeCompare(remoteBranchLabel(right.ref), undefined, { numeric: true, sensitivity: 'base' }))
    .map((branch) => branch.ref)
  $: remoteScopeByLabel = new Map(orderedRemoteScopes.map((remoteScope) => [remoteBranchLabel(remoteScope), remoteScope]))
  $: filteredRemoteScopes = matchingBranches([...remoteScopeByLabel.keys()], query).map((label) => remoteScopeByLabel.get(label) as string)
  $: selectedScope = scope
  $: selectedLabel = allBranches
    ? allLabel
    : selectedScope === 'HEAD' && currentDetached
      ? `Detached · ${currentHead.slice(0, 8) || 'HEAD'}`
      : remoteBranchLabel(selectedScope)
  $: selectedRemoteUnavailable = !allBranches && remoteScopeUnavailable(selectedScope, remoteCatalog)
  $: normalizedQuery = query.trim().toLocaleLowerCase()
  $: showSelectedRemoteUnavailable = selectedRemoteUnavailable
    && (!normalizedQuery || remoteBranchLabel(selectedScope).toLocaleLowerCase().includes(normalizedQuery))
  $: showAll = !normalizedQuery || allLabel.toLocaleLowerCase().includes(normalizedQuery)
  $: detachedHeadLabel = `Detached HEAD ${currentHead.slice(0, 8) || ''}`.trim()
  $: showDetachedHead = currentDetached && (!normalizedQuery || detachedHeadLabel.toLocaleLowerCase().includes(normalizedQuery))
  $: pinnedOption = !normalizedQuery && pinnedScope && filteredOptions.includes(pinnedScope) ? pinnedScope : ''
  $: pinnedRemoteOption = !normalizedQuery
    && !allBranches
    && isRemoteBranchScope(selectedScope)
    && filteredRemoteScopes.includes(selectedScope)
    && !filteredRemoteScopes.slice(0, pageSize).includes(selectedScope)
    ? selectedScope
    : ''
  $: visiblePageOptions = filteredOptions.slice(0, visibleCount)
  $: visibleRemotePageOptions = filteredRemoteScopes.slice(0, visibleRemoteCount)
  $: keyboardLocalOption = keyboardScope
    && !isRemoteBranchScope(keyboardScope)
    && keyboardScope !== pinnedOption
    && filteredOptions.includes(keyboardScope)
    && !visiblePageOptions.includes(keyboardScope)
    ? keyboardScope
    : ''
  $: keyboardRemoteOption = keyboardScope
    && isRemoteBranchScope(keyboardScope)
    && keyboardScope !== pinnedRemoteOption
    && filteredRemoteScopes.includes(keyboardScope)
    && !visibleRemotePageOptions.includes(keyboardScope)
    ? keyboardScope
    : ''
  $: visibleOptions = visiblePageOptions.filter((branch) => branch !== pinnedOption && branch !== keyboardLocalOption)
  $: visibleRemoteOptions = visibleRemotePageOptions.filter((branch) => branch !== pinnedRemoteOption && branch !== keyboardRemoteOption)
  $: renderedOptions = [...(pinnedOption ? [pinnedOption] : []), ...visibleOptions, ...(keyboardLocalOption ? [keyboardLocalOption] : [])]
  $: renderedRemoteOptions = [...(pinnedRemoteOption ? [pinnedRemoteOption] : []), ...visibleRemoteOptions, ...(keyboardRemoteOption ? [keyboardRemoteOption] : [])]
  $: optionScopes = [
    ...(showAll ? [allChoice] : []),
    ...(showDetachedHead ? [detachedChoice] : []),
    ...renderedOptions,
    ...(showSelectedRemoteUnavailable ? [selectedScope] : []),
    ...renderedRemoteOptions,
  ]
  $: activeDescendant = activeScope && optionScopes.includes(activeScope) ? optionID(activeScope) : undefined
  $: remoteLoading = remoteCatalog.some((entry) => entry.loading)
  $: remoteErrors = remoteCatalog.filter((entry) => entry.error)
  $: remoteReadsComplete = remoteCatalog.length > 0 && remoteCatalog.every((entry) => entry.loaded || entry.error)

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
    visibleRemoteCount = pageSize
    pinnedScope = !allBranches
      && !isRemoteBranchScope(selectedScope)
      && filteredOptions.includes(selectedScope)
      && !filteredOptions.slice(0, pageSize).includes(selectedScope)
      ? selectedScope
      : ''
    activeScope = allBranches ? allChoice : currentDetached && selectedScope === 'HEAD' ? detachedChoice : selectedScope
    onOpen()
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
    visibleRemoteCount = pageSize
    activeScope = ''
    pinnedScope = ''
    keyboardScope = ''
  }

  function resetQuery(event: Event): void {
    const nextQuery = (event.currentTarget as HTMLInputElement).value
    const nextPattern = nextQuery.trim().toLocaleLowerCase()
    const nextMatches = matchingBranches(orderedOptions, nextQuery)
    const nextRemoteLabels = matchingBranches([...remoteScopeByLabel.keys()], nextQuery)
    visibleCount = pageSize
    visibleRemoteCount = pageSize
    keyboardScope = ''
    activeScope = !nextPattern || allLabel.toLocaleLowerCase().includes(nextPattern)
      ? allChoice
      : currentDetached && detachedHeadLabel.toLocaleLowerCase().includes(nextPattern)
        ? detachedChoice
        : nextMatches[0] ?? remoteScopeByLabel.get(nextRemoteLabels[0] ?? '') ?? ''
  }

  function select(nextScope: string, nextAllBranches = false): void {
    scope = nextScope
    allBranches = nextAllBranches
    close(true)
    onChange(nextScope, nextAllBranches)
  }

  function handleScroll(event: Event): void {
    if (visibleCount >= filteredOptions.length && visibleRemoteCount >= filteredRemoteScopes.length) return
    const target = event.currentTarget as HTMLDivElement
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight
    if (remaining > target.clientHeight / 2) return
    visibleCount = nextVisibleBranchCount(visibleCount, filteredOptions.length, pageSize)
    visibleRemoteCount = nextVisibleBranchCount(visibleRemoteCount, filteredRemoteScopes.length, pageSize)
  }

  async function handleSearchKeydown(event: KeyboardEvent): Promise<void> {
    if (event.key === 'Tab') {
      if (!event.shiftKey && remoteErrors.length > 0) return
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
    const choices = [
      ...(showAll ? [allChoice] : []),
      ...(showDetachedHead ? [detachedChoice] : []),
      ...filteredOptions,
      ...(showSelectedRemoteUnavailable ? [selectedScope] : []),
      ...filteredRemoteScopes,
    ]
    if (choices.length === 0) return
    const nextScope = nextBranchChoice(choices, activeScope, event.key as BranchNavigationKey)
    const nextBranchIndex = filteredOptions.indexOf(nextScope)
    const nextRemoteIndex = filteredRemoteScopes.indexOf(nextScope)
    const canRevealNextPage = event.key === 'ArrowDown' || event.key === 'PageDown'
    if (canRevealNextPage && nextBranchIndex >= visibleCount && nextBranchIndex < visibleCount + pageSize) {
      visibleCount = nextVisibleBranchCount(visibleCount, filteredOptions.length, pageSize)
      keyboardScope = ''
    } else if (canRevealNextPage && nextRemoteIndex >= visibleRemoteCount && nextRemoteIndex < visibleRemoteCount + pageSize) {
      visibleRemoteCount = nextVisibleBranchCount(visibleRemoteCount, filteredRemoteScopes.length, pageSize)
      keyboardScope = ''
    } else if ((nextBranchIndex >= visibleCount && nextScope !== pinnedOption) || (nextRemoteIndex >= visibleRemoteCount && nextScope !== pinnedRemoteOption)) {
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

  function handleRetryKeydown(event: KeyboardEvent, last: boolean): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      close(true)
      return
    }
    if (event.key === 'Tab' && !event.shiftKey && last) close(false)
  }
</script>

<div class="branch-scope-picker" bind:this={root}>
  <button
    bind:this={trigger}
    class:open
    class="scope-picker-trigger branch-scope-trigger"
    type="button"
    {disabled}
    aria-label={`Branch scope: ${selectedLabel}${selectedRemoteUnavailable ? ' unavailable' : ''}`}
    aria-haspopup="dialog"
    aria-expanded={open}
    on:click={() => void toggle()}
  >
    <span class="scope-trigger-copy"><small>Branch</small><strong>{selectedLabel}</strong></span>
    {#if selectedRemoteUnavailable}<b class="scope-badge unavailable">Unavailable</b>{/if}
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
          aria-label="Search branches"
          aria-autocomplete="list"
          aria-controls={listboxID}
          aria-expanded="true"
          aria-activedescendant={activeDescendant}
          on:input={resetQuery}
          on:keydown={(event) => void handleSearchKeydown(event)}
        />
      </div>
      <div id={listboxID} class="branch-scope-list" role="listbox" aria-label="Branches" on:scroll={handleScroll}>
        {#if showAll}
          <button id={optionID(allChoice)} class:selected={allBranches} class:keyboard-active={activeScope === allChoice} type="button" role="option" aria-selected={allBranches} tabindex="-1" on:mouseenter={() => (activeScope = allChoice)} on:click={() => select(scope, true)}>
            <span class="scope-option-copy"><strong>{allLabel}</strong><small>{allDescription}</small></span>
          </button>
        {/if}
        {#if showDetachedHead}
          <button id={optionID(detachedChoice)} class:selected={!allBranches && selectedScope === 'HEAD'} class:keyboard-active={activeScope === detachedChoice} type="button" role="option" aria-selected={!allBranches && selectedScope === 'HEAD'} tabindex="-1" on:mouseenter={() => (activeScope = detachedChoice)} on:click={() => select('HEAD')}>
            <span class="scope-option-copy"><strong>Detached HEAD</strong><small>{currentHead.slice(0, 8) || 'unborn'}</small></span>
            <span class="scope-option-badges"><b class="scope-badge current">Current</b></span>
          </button>
        {/if}

        {#if renderedOptions.length}<div class="scope-option-group" role="presentation">Local branches</div>{/if}
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

        {#if remotes.length || selectedRemoteUnavailable}<div class="scope-option-group remote" role="presentation">Remote branches</div>{/if}
        {#if showSelectedRemoteUnavailable}
          <button id={optionID(selectedScope)} class="selected" class:keyboard-active={activeScope === selectedScope} type="button" role="option" aria-selected="true" tabindex="-1" on:mouseenter={() => (activeScope = selectedScope)} on:click={() => select(selectedScope)}>
            <span class="scope-option-copy"><strong>{remoteBranchLabel(selectedScope)}</strong><small>The fetched ref is no longer available.</small></span>
            <span class="scope-option-badges"><b class="scope-badge unavailable">Unavailable</b></span>
          </button>
        {/if}
        {#each renderedRemoteOptions as remoteScope (remoteScope)}
          {@const branch = remoteBranchByRef.get(remoteScope)}
          {@const badge = resolveRefBadge(remoteBranchLabel(remoteScope), remotes, remoteBadgeRules)}
          <button id={optionID(remoteScope)} class:selected={!allBranches && selectedScope === remoteScope} class:keyboard-active={activeScope === remoteScope} type="button" role="option" aria-selected={!allBranches && selectedScope === remoteScope} tabindex="-1" on:mouseenter={() => (activeScope = remoteScope)} on:click={() => select(remoteScope)}>
            <span class="scope-option-copy remote"><strong><RemoteBadgeIcon name={badge.icon} size={15} />{remoteBranchLabel(remoteScope)}</strong><small>Read-only fetched ref</small></span>
            <span class="scope-option-badges">{#if branch?.default}<b class="scope-badge default">Default</b>{/if}<b class="scope-badge muted">Remote</b></span>
          </button>
        {/each}
      </div>
      {#if remoteLoading}<p class="scope-picker-inline-state" role="status">Loading remote branches…</p>{/if}
      {#each remoteErrors as entry, index (entry.remote.name)}
        <div class="scope-picker-inline-state error" role="alert"><span>{entry.remote.name}: {entry.error}</span><button type="button" on:click={() => onRetryRemote(entry.remote.name)} on:keydown={(event) => handleRetryKeydown(event, index === remoteErrors.length - 1)}>Retry</button></div>
      {/each}
      {#if remotes.length && remoteReadsComplete && !remoteLoading && remoteErrors.length === 0 && remoteBranches.length === 0}<p class="scope-picker-inline-state" role="status">No fetched remote branches</p>{/if}
      {#if filteredOptions.length === 0 && filteredRemoteScopes.length === 0 && !showSelectedRemoteUnavailable && !showAll && !remoteLoading}
        <p class="scope-picker-empty" role="status" aria-live="polite">No branches match “{query.trim()}”</p>
      {:else if visibleCount < filteredOptions.length || visibleRemoteCount < filteredRemoteScopes.length}
        <p class="scope-picker-status" role="status" aria-live="polite">{Math.min(visibleCount, filteredOptions.length)} local · {Math.min(visibleRemoteCount, filteredRemoteScopes.length)} remote shown</p>
      {/if}
    </div>
  {/if}
</div>
