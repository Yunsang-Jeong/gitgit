<script lang="ts">
  import type { NavigatorView } from '../lib/types'

  export let repositoryOpen = false
  export let view: NavigatorView = 'commit'
  export let searching = false
  export let scope = 'HEAD'
  export let scanned = 0
  export let count = 0
  export let loaded = 0
  export let message = ''
  export let kind: 'info' | 'success' | 'warning' | 'error' = 'info'
  export let version = 'dev'
  export let onCancel: () => void
</script>

<footer class="status-bar">
  <div class:error={Boolean(message) && kind === 'error'} class="status-copy" role={kind === 'error' ? 'alert' : 'status'} aria-live={kind === 'error' ? 'assertive' : 'polite'}>
    {#if message}
      <span aria-hidden="true">{kind === 'success' ? '✓' : kind === 'info' ? 'i' : '!'}</span><span>{message}</span>
    {:else if repositoryOpen}
      {#if view === 'worktrees'}
        <strong>Worktrees</strong><span>·</span><span>{count.toLocaleString()} registered</span>
      {:else if view === 'search'}
        <strong>{scope || 'HEAD'}</strong>
        <span>·</span>
        <span>{scanned.toLocaleString()} commits scanned</span>
        <span>·</span>
        <span>{count.toLocaleString()} matching commits</span>
      {:else}
        <strong>{scope || 'HEAD'}</strong>
        <span>·</span>
        <span>{loaded.toLocaleString()} of {scanned.toLocaleString()} commits loaded</span>
        <span>·</span>
        <span>{count.toLocaleString()} visible</span>
      {/if}
    {:else}
      <span>No repository open</span>
    {/if}
  </div>
  <div class="status-spacer"></div>
  {#if searching}
    <div class="search-progress"><span></span></div>
    <button type="button" on:click={onCancel}>Cancel</button>
  {/if}
  <span class="status-version" title={`GitGit build ${version}`}>v{version}</span>
</footer>
