<script lang="ts">
  import { onMount } from 'svelte'
  import { sparsePlan } from '../lib/sparse-plan'
  import { findSparseNode, flattenSparseNodes, selectedSparsePaths, setSparseNodeChildren, toSparseNodes, toggleSparseExpansion, toggleSparseNode, type SparseTreeNode } from '../lib/sparse-tree'
  import { sparseDirectories, sparseSummary, worktreeName } from '../lib/worktree-actions'
  import type { RepositoryTreeResponse, WorktreeInfo } from '../lib/types'

  export let worktree: WorktreeInfo
  export let busy = false
  export let onLoadTree: (revision: string, directory: string) => Promise<RepositoryTreeResponse>
  // Each returns an empty string on success, or the message to show in place.
  export let onApply: (action: 'set' | 'expand' | 'contract', directories: string[]) => Promise<string>
  export let onDisable: () => Promise<string>
  export let onClose: () => void

  let nodes: SparseTreeNode[] = []
  let loading = true
  let loadingPaths = new Set<string>()
  let error = ''
  let confirmingHide = false

  $: current = sparseDirectories(worktree)
  $: selection = selectedSparsePaths(nodes)
  $: plan = sparsePlan(worktree.sparse, selection)
  $: visible = flattenSparseNodes(nodes)
  // Contracting is the only direction that takes files away, so it is the only
  // one that asks twice.
  $: hides = plan.removed.length > 0
  $: canApply = !busy && !loading && plan.action !== 'none' && selection.length > 0

  onMount(() => {
    void loadLevel('')
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  })

  // The tree is read from this worktree's own HEAD, which is the tree the
  // backend validates new directories against.
  async function loadLevel(directory: string): Promise<SparseTreeNode[] | null> {
    try {
      const response = await onLoadTree(worktree.head, directory)
      return toSparseNodes(response.entries, current)
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause)
      return null
    }
  }

  async function initializeRoot(): Promise<void> {
    const roots = await loadLevel('')
    if (roots) nodes = roots
    loading = false
  }

  $: if (loading && worktree.head) void initializeRoot()

  async function toggleExpansion(node: SparseTreeNode): Promise<void> {
    if (node.expanded) {
      nodes = toggleSparseExpansion(nodes, node.path, false)
      return
    }
    if (node.children === null) {
      loadingPaths = new Set(loadingPaths).add(node.path)
      const children = await loadLevel(node.path)
      loadingPaths = new Set([...loadingPaths].filter((path) => path !== node.path))
      if (!children) return
      nodes = setSparseNodeChildren(nodes, node.path, children)
    }
    nodes = toggleSparseExpansion(nodes, node.path, true)
  }

  function toggleChecked(node: SparseTreeNode, checked: boolean): void {
    confirmingHide = false
    error = ''
    nodes = toggleSparseNode(nodes, node.path, checked)
  }

  async function apply(): Promise<void> {
    if (!canApply || plan.action === 'none') return
    if (hides && !confirmingHide) {
      confirmingHide = true
      return
    }
    error = ''
    const failure = await onApply(plan.action, plan.directories)
    if (failure) {
      error = failure
      confirmingHide = false
    } else {
      onClose()
    }
  }

  async function disable(): Promise<void> {
    if (busy) return
    error = ''
    const failure = await onDisable()
    if (failure) error = failure
    else onClose()
  }
</script>

<div class="worktree-confirm-backdrop" role="presentation" on:mousedown={() => { if (!busy) onClose() }}>
  <div
    class="worktree-confirm worktree-dialog"
    role="dialog"
    aria-modal="true"
    aria-labelledby="sparse-worktree-title"
    tabindex="-1"
    on:mousedown|stopPropagation
  >
    <h2 id="sparse-worktree-title">Sparse-checkout · {worktreeName(worktree)}</h2>
    <p>Choose the directories to keep on disk. Everything under a selected directory is included.</p>

    <dl class="worktree-dialog-preview">
      <dt>Now</dt>
      <dd>{sparseSummary(worktree)}</dd>
    </dl>

    <div class="sparse-tree" role="tree" aria-label="Repository directories">
      {#if loading}
        <p class="sparse-tree-state">Reading directories…</p>
      {:else if visible.length === 0}
        <p class="sparse-tree-state">This worktree has no directories to select.</p>
      {:else}
        {#each visible as entry (entry.node.path)}
          {@const node = entry.node}
          <div class="sparse-tree-row" style={`padding-left: ${6 + entry.depth * 16}px`} role="treeitem" aria-level={entry.depth + 1} aria-selected={node.checked} aria-expanded={node.expanded}>
            <button
              class="sparse-tree-chevron"
              type="button"
              aria-label={node.expanded ? `Collapse ${node.name}` : `Expand ${node.name}`}
              disabled={busy || loadingPaths.has(node.path)}
              on:click={() => void toggleExpansion(node)}
            >
              <svg class:expanded={node.expanded} viewBox="0 0 16 16" aria-hidden="true"><path d="m6 4 4 4-4 4" /></svg>
            </button>
            <label>
              <input type="checkbox" checked={node.checked} disabled={busy} on:change={(event) => toggleChecked(node, event.currentTarget.checked)} />
              <span>{node.name}</span>
            </label>
            {#if loadingPaths.has(node.path)}<small>loading…</small>{/if}
          </div>
        {/each}
      {/if}
    </div>

    {#if plan.action === 'none'}
      <p class="worktree-dialog-hint">{selection.length === 0 ? 'Select at least one directory, or disable sparse-checkout entirely.' : 'This selection matches the current one.'}</p>
    {:else if confirmingHide}
      <p class="worktree-dialog-error" role="alert">
        Applying hides {plan.removed.join(', ')} from this worktree. Choose Apply again to continue.
      </p>
    {:else if hides}
      <p class="worktree-dialog-hint">Applying will hide {plan.removed.join(', ')}.</p>
    {/if}

    {#if error}<p class="worktree-dialog-error" role="alert">{error}</p>{/if}

    <div class="worktree-confirm-actions sparse-actions">
      <button class="danger" type="button" disabled={busy || !worktree.sparse.enabled} on:click={() => void disable()}>Disable sparse-checkout</button>
      <span class="sparse-actions-spacer"></span>
      <button type="button" disabled={busy} on:click={onClose}>Cancel</button>
      <button class="primary" type="button" disabled={!canApply} on:click={() => void apply()}>
        {busy ? 'Applying…' : confirmingHide ? 'Apply anyway' : 'Apply'}
      </button>
    </div>
  </div>
</div>
