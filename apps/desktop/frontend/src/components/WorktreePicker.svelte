<script lang="ts">
  import { onMount, tick } from 'svelte'
  import type { WorktreeInfo } from '../lib/types'

  export let worktrees: WorktreeInfo[] = []
  export let activeRoot = ''
  export let projectRoot = ''
  export let disabled = false
  export let onChange: (worktree: WorktreeInfo) => void

  let open = false
  let root: HTMLDivElement
  let trigger: HTMLButtonElement
  let activePath = ''

  $: orderedWorktrees = [...worktrees].sort(compareWorktrees)
  $: activeWorktree = worktrees.find((worktree) => worktree.path === activeRoot)

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

  function compareWorktrees(left: WorktreeInfo, right: WorktreeInfo): number {
    const leftPriority = left.path === activeRoot ? 0 : left.path === projectRoot ? 1 : 2
    const rightPriority = right.path === activeRoot ? 0 : right.path === projectRoot ? 1 : 2
    return leftPriority - rightPriority || worktreeLabel(left).localeCompare(worktreeLabel(right), undefined, { numeric: true, sensitivity: 'base' }) || left.path.localeCompare(right.path)
  }

  function worktreeLabel(worktree: WorktreeInfo | undefined): string {
    if (!worktree) return 'Unavailable'
    return worktree.detached ? `Detached · ${shortHead(worktree.head)}` : worktree.branch || 'Unknown branch'
  }

  function shortHead(head: string): string {
    return head ? head.slice(0, 8) : 'unborn'
  }

  function unavailable(worktree: WorktreeInfo): boolean {
    return worktree.bare || worktree.prunable
  }

  async function toggle(): Promise<void> {
    open = !open
    if (!open) return
    activePath = activeRoot || orderedWorktrees[0]?.path || ''
    await tick()
    focusActiveOption()
  }

  function close(restoreFocus: boolean): void {
    open = false
    if (restoreFocus) void tick().then(() => trigger?.focus())
  }

  function select(worktree: WorktreeInfo): void {
    if (unavailable(worktree)) return
    close(true)
    if (worktree.path !== activeRoot) onChange(worktree)
  }

  function handleListKeydown(event: KeyboardEvent): void {
    if (event.key === 'Tab') {
      close(false)
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End', 'Enter'].includes(event.key)) return
    event.preventDefault()
    const enabled = orderedWorktrees.filter((worktree) => !unavailable(worktree))
    if (event.key === 'Enter') {
      const selected = enabled.find((worktree) => worktree.path === activePath)
      if (selected) select(selected)
      return
    }
    const currentIndex = Math.max(0, enabled.findIndex((worktree) => worktree.path === activePath))
    const nextIndex = event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? enabled.length - 1
        : event.key === 'ArrowDown'
          ? Math.min(enabled.length - 1, currentIndex + 1)
          : Math.max(0, currentIndex - 1)
    activePath = enabled[nextIndex]?.path ?? ''
    focusActiveOption()
  }

  function focusActiveOption(): void {
    const option = root?.querySelector<HTMLButtonElement>(`[data-worktree-path="${CSS.escape(activePath)}"]`)
    option?.focus()
  }
</script>

<div class="worktree-picker" bind:this={root}>
  <button
    bind:this={trigger}
    class:open
    class="scope-picker-trigger worktree-picker-trigger"
    type="button"
    {disabled}
    aria-label={`Worktree: ${worktreeLabel(activeWorktree)}`}
    aria-haspopup="listbox"
    aria-expanded={open}
    on:click={() => void toggle()}
  >
    <span class="scope-trigger-copy"><small>Worktree</small><strong>{worktreeLabel(activeWorktree)}</strong></span>
    <svg class="scope-picker-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
  </button>

  {#if open}
    <div
      class="worktree-picker-menu"
      role={orderedWorktrees.length > 0 ? 'listbox' : undefined}
      aria-label={orderedWorktrees.length > 0 ? 'Select worktree' : undefined}
      tabindex="-1"
      on:keydown={handleListKeydown}
    >
      {#if orderedWorktrees.length === 0}
        <p class="scope-picker-empty" role="status">No worktrees available</p>
      {:else}
        {#each orderedWorktrees as worktree (worktree.path)}
          <button
            class:selected={worktree.path === activeRoot}
            class:keyboard-active={worktree.path === activePath}
            type="button"
            role="option"
            aria-selected={worktree.path === activeRoot}
            tabindex={worktree.path === activePath ? 0 : -1}
            disabled={unavailable(worktree)}
            data-worktree-path={worktree.path}
            title={worktree.path}
            on:focus={() => (activePath = worktree.path)}
            on:click={() => select(worktree)}
          >
            <span class="scope-option-copy"><strong>{worktreeLabel(worktree)}</strong><small>{worktree.path}</small></span>
            <span class="scope-option-badges">
              {#if worktree.path === activeRoot}<b class="scope-badge current">Current</b>{/if}
              {#if worktree.path === projectRoot}<b class="scope-badge main">Main</b>{/if}
              {#if worktree.dirty}<b class="scope-badge warning">Changes</b>{/if}
              {#if worktree.locked}<b class="scope-badge">Locked</b>{/if}
              {#if worktree.sparse.enabled}<b class="scope-badge">Sparse</b>{/if}
              {#if worktree.prunable}<b class="scope-badge warning">Prunable</b>{/if}
              {#if worktree.bare}<b class="scope-badge warning">Bare</b>{/if}
            </span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>
