<script lang="ts">
  import BranchScopePicker from './BranchScopePicker.svelte'
  import ProjectSwitcher from './ProjectSwitcher.svelte'
  import ResultsTable from './ResultsTable.svelte'
  import SearchComposer from './SearchComposer.svelte'
  import SearchCriteriaBar from './SearchCriteriaBar.svelte'
  import WorktreePicker from './WorktreePicker.svelte'
  import type {
    Pattern,
    RegisteredProject,
    RepositoryState,
    SearchResult,
    SearchSessionSummary,
    WorktreeInfo,
  } from '../lib/types'

  export let sessions: SearchSessionSummary[] = []
  export let activeSessionID = ''
  export let repository: RepositoryState | null = null
  export let projects: RegisteredProject[] = []
  export let activeProjectRoot = ''
  export let branches: string[] = []
  export let patterns: Pattern[] = []
  export let engine = 'glob'
  export let scope = 'HEAD'
  export let allRefs = false
  export let author = ''
  export let since = ''
  export let until = ''
  export let results: SearchResult[] = []
  export let selectedIndex = -1
  export let searching = false
  export let hasSearched = false
  export let stale = false
  export let applied = false
  export let scanned = 0
  export let error = ''
  export let disabled = false
  export let onNewSession: () => void
  export let onSelectSession: (id: string) => void
  export let onRegisterProject: () => void
  export let onSelectProject: (project: RegisteredProject) => void
  export let onToggleProjectFavorite: (project: RegisteredProject) => void
  export let onWorktreeChange: (worktree: WorktreeInfo) => void
  export let onScopeChange: (scope: string, allRefs: boolean) => void
  export let onRunSearch: () => void
  export let onCancelSearch: () => void
  export let onRemovePattern: (index: number) => void

  $: activeWorktree = repository?.worktrees.find((worktree) => worktree.path === repository?.root)
</script>

<section class="search-workspace pane">
  <aside class="search-session-sidebar">
    <header>
      <div><strong>Search sessions</strong><span>{sessions.length}</span></div>
      <button type="button" on:click={onNewSession} aria-label="New search session" title="New search session">＋</button>
    </header>
    <div class="search-session-list" role="listbox" aria-label="Search sessions">
      {#each sessions as session}
        <button
          class:active={session.id === activeSessionID}
          class="search-session-item"
          type="button"
          role="option"
          aria-selected={session.id === activeSessionID}
          on:click={() => onSelectSession(session.id)}
        >
          <span class="search-session-item-title"><strong>{session.title}</strong><b class="status-{session.status}">{session.status}</b></span>
          <span>{session.project || 'Select project'}</span>
          <small>{session.query || 'No conditions yet'}</small>
          {#if session.result_count > 0}<em>{session.result_count.toLocaleString()} commits</em>{/if}
        </button>
      {/each}
    </div>
  </aside>

  <section class="search-session-main">
    <header class="search-session-toolbar" aria-label="Search target controls">
      <ProjectSwitcher
        {projects}
        {activeProjectRoot}
        {disabled}
        showFavorites={false}
        onRegister={onRegisterProject}
        onSelect={onSelectProject}
        onToggleFavorite={onToggleProjectFavorite}
      />
      <WorktreePicker
        worktrees={repository?.worktrees ?? []}
        activeRoot={repository?.root ?? ''}
        projectRoot={activeProjectRoot}
        disabled={disabled || !repository}
        onChange={onWorktreeChange}
      />
      <BranchScopePicker
        {scope}
        allBranches={allRefs}
        {branches}
        worktrees={repository?.worktrees ?? []}
        defaultBranch={repository?.default_branch ?? ''}
        currentBranch={repository?.branch ?? ''}
        currentDetached={Boolean(activeWorktree?.detached)}
        currentHead={repository?.head ?? ''}
        activeWorktreeRoot={repository?.root ?? ''}
        disabled={disabled || !repository}
        onChange={onScopeChange}
      />
      <span class="search-session-toolbar-spacer"></span>
      {#if searching}
        <span class="search-session-progress">Scanning…</span>
        <button class="history-action" type="button" on:click={onCancelSearch}>Cancel</button>
      {:else if hasSearched}
        <span class="search-session-progress">{scanned.toLocaleString()} scanned · {results.length.toLocaleString()} commits</span>
      {/if}
      <button class="history-action history-run-search" type="button" on:click={onRunSearch} disabled={disabled || !repository || searching || patterns.length === 0}>
        {searching ? 'Searching…' : 'Search'}
      </button>
    </header>

    <div class="search-session-query">
      <SearchComposer bind:patterns bind:engine bind:scope bind:allRefs bind:author bind:since bind:until onSearch={onRunSearch} />
    </div>

    <SearchCriteriaBar {patterns} {stale} {applied} onRemove={onRemovePattern} />

    <ResultsTable
      bind:selectedIndex
      {results}
      filteredOut={false}
      hasPreviousResults={results.length > 0}
      {searching}
      {hasSearched}
      {error}
      defaultBranch={repository?.default_branch ?? ''}
      onRetry={onRunSearch}
    />
  </section>
</section>
