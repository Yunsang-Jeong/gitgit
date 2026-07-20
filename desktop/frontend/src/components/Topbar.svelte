<script lang="ts">
  import ProjectSwitcher from './ProjectSwitcher.svelte'
  import type { NavigatorView, RegisteredProject, RepositoryState } from '../lib/types'

  export let repository: RepositoryState | null
  export let projects: RegisteredProject[] = []
  export let activeProjectRoot = ''
  export let refreshing = false
  export let syncing = false
  export let transitioning = false
  export let view: NavigatorView = 'commit'
  export let onRegisterProject: () => void
  export let onSelectProject: (project: RegisteredProject) => void
  export let onToggleProjectFavorite: (project: RegisteredProject) => void
  export let onRefresh: () => void
  export let onSync: () => void
  export let onOpenSettings: () => void
  export let onViewChange: (view: NavigatorView) => void
</script>

<header class="topbar">
  <div class="brand">GitGit</div>

  <nav class="topbar-view-switcher" aria-label="Repository views">
    <button class:selected={view === 'commit'} type="button" on:click={() => onViewChange('commit')}>Commit</button>
    <button class:selected={view === 'worktrees'} type="button" on:click={() => onViewChange('worktrees')}>Worktree</button>
    <button class:selected={view === 'search'} type="button" on:click={() => onViewChange('search')}>Search</button>
  </nav>

  {#if view === 'search'}
    <div class="topbar-workspace-context"><span>Workspace</span><strong>Search sessions</strong></div>
  {:else}
    <ProjectSwitcher
      {projects}
      {activeProjectRoot}
      disabled={transitioning}
      onRegister={onRegisterProject}
      onSelect={onSelectProject}
      onToggleFavorite={onToggleProjectFavorite}
    />
  {/if}

  <button class="toolbar-action remote-sync-action" class:spinning={syncing} type="button" on:click={onSync} disabled={!repository || syncing || refreshing || transitioning} title="Fetch all remotes and prune deleted refs">
    <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M5.2 12.4h-1A2.7 2.7 0 0 1 4 7a4.4 4.4 0 0 1 8.4 1.2A2.2 2.2 0 0 1 12 12.5h-1.2M8 7.1v6.1m0 0-2.1-2.1M8 13.2l2.1-2.1" /></svg>
    <span>{syncing ? 'Syncing…' : 'Sync'}</span>
  </button>

  <button class="toolbar-action" class:spinning={refreshing} type="button" on:click={onRefresh} disabled={!repository || refreshing || syncing || transitioning}>
    <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M13 4.5V1.8m0 2.7h-2.7M3 11.5v2.7m0-2.7h2.7M3.8 5.1A5.2 5.2 0 0 1 13 4.5M12.2 10.9A5.2 5.2 0 0 1 3 11.5" /></svg>
    <span>Refresh</span>
  </button>

  <button class="toolbar-action settings-action" type="button" on:click={onOpenSettings} title="Open Settings (⌘,)">
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="2.2" /><path d="M8 1.7v1.4m0 9.8v1.4M1.7 8h1.4m9.8 0h1.4M3.6 3.6l1 1m6.8 6.8 1 1m0-8.8-1 1m-6.8 6.8-1 1" /></svg>
    <span>Settings</span>
  </button>
</header>
