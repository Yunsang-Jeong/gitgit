<script lang="ts">
  import ProjectSwitcher from './ProjectSwitcher.svelte'
  import type { NavigatorView, RegisteredProject, RepositoryState } from '../lib/types'

  export let repository: RepositoryState | null
  export let projects: RegisteredProject[] = []
  export let activeProjectRoot = ''
  export let refreshing = false
  export let syncing = false
  export let pulling = false
  export let transitioning = false
  export let view: NavigatorView = 'commit'
  export let onRegisterProject: () => void
  export let onSelectProject: (project: RegisteredProject) => void
  export let onToggleProjectFavorite: (project: RegisteredProject) => void
  export let onUnregisterProject: (project: RegisteredProject) => void
  export let onRefresh: () => void
  export let onSync: () => void
  export let onPull: () => void
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

  {#if view !== 'search'}
    <ProjectSwitcher
      {projects}
      {activeProjectRoot}
      disabled={transitioning}
      showFavorites={false}
      onRegister={onRegisterProject}
      onSelect={onSelectProject}
      onToggleFavorite={onToggleProjectFavorite}
      onUnregister={onUnregisterProject}
    />
  {/if}

  <span class="topbar-spacer" aria-hidden="true"></span>

  <button class="toolbar-action remote-sync-action" class:spinning={syncing} type="button" on:click={onSync} disabled={view !== 'commit' || !repository || syncing || pulling || refreshing || transitioning} title={view === 'commit' ? 'Fetch all remotes and prune deleted refs' : 'Sync is available in Commit'}>
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="3.25" cy="3.25" r="1.2" /><circle cx="12.75" cy="3.25" r="1.2" /><circle cx="8" cy="12.75" r="1.2" /><path d="M3.25 4.45v.65a3.55 3.55 0 0 0 3.55 3.55h2.4a3.55 3.55 0 0 0 3.55-3.55v-.65M8 5.5v4.9m0 0-1.75-1.75M8 10.4l1.75-1.75" /></svg>
    <span>{syncing ? 'Syncing…' : 'Sync'}</span>
  </button>

  <button class="toolbar-action remote-pull-action" class:spinning={pulling} type="button" on:click={onPull} disabled={view !== 'commit' || !repository || pulling || syncing || refreshing || transitioning} title={view === 'commit' ? 'Fast-forward the current tracking branch' : 'Pull is available in Commit'}>
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="3.25" cy="3.25" r="1.2" /><circle cx="3.25" cy="12.75" r="1.2" /><circle cx="12.75" cy="12.75" r="1.2" /><path d="M3.25 4.45v3.3a3.55 3.55 0 0 0 3.55 3.55h4.75M8 3.45v4.9m0 0-1.75-1.75M8 8.35l1.75-1.75" /></svg>
    <span>{pulling ? 'Pulling…' : 'Pull'}</span>
  </button>

  <button class="toolbar-action" class:spinning={refreshing} type="button" on:click={onRefresh} disabled={view === 'search' || !repository || refreshing || syncing || pulling || transitioning} title={view === 'search' ? 'Search sessions keep their own results' : 'Refresh repository state'}>
    <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M13 4.5V1.8m0 2.7h-2.7M3 11.5v2.7m0-2.7h2.7M3.8 5.1A5.2 5.2 0 0 1 13 4.5M12.2 10.9A5.2 5.2 0 0 1 3 11.5" /></svg>
    <span>Refresh</span>
  </button>

  <button class="toolbar-action settings-action" type="button" on:click={onOpenSettings} title="Open Settings (⌘,)">
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="2.2" /><path d="M8 1.7v1.4m0 9.8v1.4M1.7 8h1.4m9.8 0h1.4M3.6 3.6l1 1m6.8 6.8 1 1m0-8.8-1 1m-6.8 6.8-1 1" /></svg>
    <span>Settings</span>
  </button>
</header>
