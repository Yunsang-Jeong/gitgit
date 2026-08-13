<script lang="ts">
  import type { WorktreeInfo } from '../lib/types'

  export let worktree: WorktreeInfo
  export let defaultBranch = ''
  export let main = false
  export let selected = false
  export let impliedMergeState = false
  export let onToggle: (worktree: WorktreeInfo, checked: boolean, range?: boolean) => void
  export let onView: (worktree: WorktreeInfo) => void = () => undefined
  export let onOpen: (worktree: WorktreeInfo) => void = () => undefined
  export let onOpenIDE: (worktree: WorktreeInfo) => void

  function shortHead(head: string): string {
    return head ? head.slice(0, 8) : 'unborn'
  }

  function pathHead(path: string): string {
    const cut = path.lastIndexOf('/')
    return cut > 0 ? path.slice(0, cut + 1) : ''
  }

  function pathTail(path: string): string {
    const cut = path.lastIndexOf('/')
    return cut > 0 ? path.slice(cut + 1) : path
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
    aria-pressed={main ? undefined : selected}
    title={main ? 'Main worktree is not selectable' : 'Click to toggle selection · Shift-click to select a range'}
    on:click={(event) => { if (!main) onToggle(worktree, !selected, event.shiftKey) }}
  >
    <span class="worktree-card-heading">
      <span class="branch-glyph">⑂</span>
      <strong>{worktree.detached ? 'detached' : worktree.branch || 'unknown'}</strong>
    </span>
    <span class="worktree-card-badges">
      {#if worktree.locked}<b class="lock-badge">Locked</b>{/if}
      {#if !worktree.detached}
        {#if worktree.branch === defaultBranch}
          <b class="merge-badge default">Default</b>
        {:else if !impliedMergeState}
          <b class:merged={worktree.merged_into_default} class="merge-badge">{worktree.merged_into_default ? 'Merged' : 'Unmerged'}</b>
        {/if}
      {/if}
      {#if worktree.sparse.enabled}<b class="sparse-badge" title={sparseTitle()}>Sparse</b>{/if}
    </span>
    <code class="worktree-card-path" title={worktree.path}><span class="worktree-card-path-head">{pathHead(worktree.path)}</span><span class="worktree-card-path-tail">{pathTail(worktree.path)}</span></code>
    <span class="worktree-card-meta">
      <span class:changed={worktree.dirty} class="worktree-state"><i></i>{worktree.dirty ? 'Changes' : 'Clean'}</span>
      <code>{shortHead(worktree.head)}</code>
    </span>
  </button>

  <footer class="worktree-card-footer">
    <button type="button" title={`View commits for ${worktree.branch || 'this worktree'}`} on:click={() => onView(worktree)}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="3.6" r="1.7" /><circle cx="8" cy="12.4" r="1.7" /><path d="M8 5.3v5.4" /></svg>
      Commits
    </button>
    <button type="button" title="Reveal this worktree in Finder" on:click={() => onOpen(worktree)}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M2.2 4.4h4l1.5 2h6.1v7.2h-11.6z" /></svg>
      Finder
    </button>
    <button type="button" title="Open this worktree in the configured IDE" on:click={() => onOpenIDE(worktree)}>
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M5.2 3.2H3.8a1.6 1.6 0 0 0-1.6 1.6v7.4a1.6 1.6 0 0 0 1.6 1.6h7.4a1.6 1.6 0 0 0 1.6-1.6v-1.4M8.8 2.2h5v5M7.1 8.9l6.5-6.5" /></svg>
      IDE
    </button>
  </footer>
</article>
