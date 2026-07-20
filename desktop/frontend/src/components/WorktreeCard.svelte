<script lang="ts">
  import type { WorktreeInfo } from '../lib/types'

  export let worktree: WorktreeInfo
  export let defaultBranch = ''
  export let main = false
  export let selected = false
  export let onToggle: (worktree: WorktreeInfo, checked: boolean, range?: boolean) => void
  export let onOpenIDE: (worktree: WorktreeInfo) => void

  function shortHead(head: string): string {
    return head ? head.slice(0, 8) : 'unborn'
  }

  function mergeLabel(): string {
    if (worktree.branch === defaultBranch) return 'Default'
    return worktree.merged_into_default ? 'Merged' : 'Unmerged'
  }

  function sparseTitle(): string {
    const directories = worktree.sparse.directories ?? []
    return directories.length > 0 ? `Sparse checkout: ${directories.join(', ')}` : 'Sparse checkout enabled'
  }
</script>

<article class:main class:selected class="worktree-card">
  {#if !main}
    <input
      class="worktree-selection-input"
      type="checkbox"
      checked={selected}
      aria-label={`Select ${worktree.branch || 'detached worktree'}`}
      on:click|stopPropagation
      on:change={(event) => onToggle(worktree, (event.currentTarget as HTMLInputElement).checked)}
    />
  {/if}

  <button
    class:selectable={!main}
    class="worktree-card-select"
    type="button"
    title={main ? 'Main worktree is not selectable' : 'Click to toggle selection · Shift-click to select a range'}
    on:click={(event) => { if (!main) onToggle(worktree, !selected, event.shiftKey) }}
  >
    <span class="worktree-card-heading">
      <span class="branch-glyph">⑂</span>
      <strong>{worktree.detached ? 'detached' : worktree.branch || 'unknown'}</strong>
    </span>
    <span class="worktree-card-badges">
      {#if main}<b class="main-badge">Main</b>{/if}
      {#if worktree.locked}<b class="lock-badge">Locked</b>{/if}
      {#if !worktree.detached}
        <b class:merged={worktree.merged_into_default} class:default={worktree.branch === defaultBranch} class="merge-badge">{mergeLabel()}</b>
      {/if}
      {#if worktree.sparse.enabled}<b class="sparse-badge" title={sparseTitle()}>Sparse</b>{/if}
    </span>
    <code class="worktree-card-path" title={worktree.path}>{worktree.path}</code>
    <span class="worktree-card-meta">
      <span class:changed={worktree.dirty} class="worktree-state"><i></i>{worktree.dirty ? 'Changes' : 'Clean'}</span>
      <code>{shortHead(worktree.head)}</code>
    </span>
  </button>

  <footer class="worktree-card-footer">
    <button type="button" on:click={() => onOpenIDE(worktree)}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M5.2 3.2H3.8a1.6 1.6 0 0 0-1.6 1.6v7.4a1.6 1.6 0 0 0 1.6 1.6h7.4a1.6 1.6 0 0 0 1.6-1.6v-1.4M8.8 2.2h5v5M7.1 8.9l6.5-6.5" /></svg>
      Open IDE
    </button>
  </footer>
</article>
