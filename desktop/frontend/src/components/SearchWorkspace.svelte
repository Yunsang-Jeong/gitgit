<script lang="ts">
  import BranchScopePicker from './BranchScopePicker.svelte'
  import ProjectSwitcher from './ProjectSwitcher.svelte'
  import ResultsTable from './ResultsTable.svelte'
  import SearchComposer from './SearchComposer.svelte'
  import WorktreePicker from './WorktreePicker.svelte'
  import { formatDate } from '../lib/datetime'
  import { searchExpressionError } from '../lib/search-expression'
  import type {
    Pattern,
    RegisteredProject,
    RepositoryState,
    SearchProgress,
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
  export let searchProgress: SearchProgress | null = null
  export let hasSearched = false
  export let stale = false
  export let applied = false
  export let scanned = 0
  export let error = ''
  export let disabled = false
  export let inspectorWidth = 440
  export let onNewSession: () => void
  export let onSelectSession: (id: string) => void
  export let onRenameSession: (id: string, title: string) => void
  export let onRemoveSession: (id: string) => void
  export let onRegisterProject: () => void
  export let onSelectProject: (project: RegisteredProject) => void
  export let onToggleProjectFavorite: (project: RegisteredProject) => void
  export let onWorktreeChange: (worktree: WorktreeInfo) => void
  export let onScopeChange: (scope: string, allRefs: boolean) => void
  export let onRunSearch: () => void
  export let onCancelSearch: () => void
  export let onSelectResult: (index: number) => void
  export let onStartInspectorResize: (event: MouseEvent) => void
  export let onResizeInspectorWithKeyboard: (event: KeyboardEvent) => void

  $: activeWorktree = repository?.worktrees.find((worktree) => worktree.path === repository?.root)
  let composerError = ''
  let sessionSidebarCollapsed = false
  let queryCollapsed = false
  $: expressionError = composerError || searchExpressionError(patterns)

  function selectSessionWithKeyboard(event: KeyboardEvent, id: string): void {
    if (event.key !== 'Enter' && event.key !== ' ') return
    if (event.target instanceof HTMLInputElement || event.target instanceof HTMLButtonElement) return
    event.preventDefault()
    onSelectSession(id)
  }

  function resultCountLabel(count: number): string {
    return `${Number(count).toLocaleString()} ${Number(count) === 1 ? 'commit' : 'commits'}`
  }
</script>

<section class:sessions-collapsed={sessionSidebarCollapsed} class="search-workspace pane">
  <aside class:collapsed={sessionSidebarCollapsed} class="search-session-sidebar">
    <header>
      <div class="search-session-heading"><strong>Search sessions</strong><span>{sessions.length}</span></div>
      <div class="search-session-sidebar-actions">
        <button
          class="search-session-collapse"
          type="button"
          on:click={() => (sessionSidebarCollapsed = !sessionSidebarCollapsed)}
          aria-label={sessionSidebarCollapsed ? 'Expand search sessions' : 'Collapse search sessions'}
          aria-expanded={!sessionSidebarCollapsed}
          title={sessionSidebarCollapsed ? 'Expand search sessions' : 'Collapse search sessions'}
        >{sessionSidebarCollapsed ? '›' : '‹'}</button>
        <button type="button" on:click={onNewSession} aria-label="New search session" title="New search session">＋</button>
      </div>
    </header>
    <div class="search-session-list" role="listbox" aria-label="Search sessions">
      {#each sessions as session}
        <div
          class:active={session.id === activeSessionID}
          class="search-session-item"
          role="option"
          tabindex="0"
          aria-selected={session.id === activeSessionID}
          on:click={() => onSelectSession(session.id)}
          on:keydown={(event) => selectSessionWithKeyboard(event, session.id)}
        >
          <span class="search-session-item-title">
            <input value={session.title} maxlength="80" on:click|stopPropagation on:input={(event) => onRenameSession(session.id, event.currentTarget.value)} on:blur={(event) => onRenameSession(session.id, event.currentTarget.value.trim() || 'Untitled search')} aria-label="Search session alias" title="Search session alias" />
            {#if session.status}<b class="status-{session.status}">{session.status}</b>{/if}
            <button class="search-session-remove" type="button" on:click|stopPropagation={() => onRemoveSession(session.id)} aria-label="Delete {session.title}" title="Delete search session">×</button>
          </span>
          <span>{session.project || 'Select project'}</span>
          <small>{session.query || 'No conditions yet'}</small>
          <footer>
            <time datetime={session.last_searched_at ?? ''}>{session.last_searched_at ? `Searched ${formatDate(session.last_searched_at, true)}` : 'Not searched'}</time>
            {#if session.result_count > 0}<em>{resultCountLabel(session.result_count)}</em>{/if}
          </footer>
        </div>
      {/each}
    </div>
  </aside>

  <section class="search-session-main">
    {#if !activeSessionID}
      <div class="search-session-empty">
        <span class="empty-symbol">⌕</span>
        <strong>No search sessions</strong>
        <p>Create a session when you are ready to define a query.</p>
        <button class="history-action" type="button" on:click={onNewSession}>＋ New search session</button>
      </div>
    {:else}
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
        <span class="search-session-progress">
          {searchProgress && searchProgress.total > 0
            ? `Scanning ${searchProgress.scanned.toLocaleString()} of ${searchProgress.total.toLocaleString()} commits…`
            : 'Scanning…'}
        </span>
        <button class="history-action" type="button" on:click={onCancelSearch}>Cancel</button>
      {:else if hasSearched}
        <span class="search-session-progress">{scanned.toLocaleString()} scanned · {results.length.toLocaleString()} commits</span>
      {/if}
      <button class="history-action history-run-search" type="button" on:click={onRunSearch} disabled={disabled || !repository || searching || patterns.length === 0 || Boolean(expressionError)}>
        {searching ? 'Searching…' : 'Search'}
      </button>
      <button
        class="search-layout-toggle"
        type="button"
        on:click={() => (queryCollapsed = !queryCollapsed)}
        aria-label={queryCollapsed ? 'Show query composer' : 'Hide query composer'}
        aria-controls="search-query-panel"
        aria-expanded={!queryCollapsed}
        title={queryCollapsed ? 'Show query composer' : 'Hide query composer'}
      >{queryCollapsed ? '↓' : '↑'}</button>
    </header>

    <div id="search-query-panel" class="search-session-query" hidden={queryCollapsed}>
      <SearchComposer bind:patterns bind:engine bind:scope bind:allRefs bind:author bind:since bind:until bind:queryError={composerError} {stale} {applied} onSearch={onRunSearch} />
    </div>

    <div class="search-results-layout" style:--search-inspector-width={`${inspectorWidth}px`}>
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
        onSelect={onSelectResult}
      />
      <button class="pane-resizer search-results-resizer" type="button" aria-label="Resize Search Inspector" title="Drag to resize Search Inspector" on:mousedown={onStartInspectorResize} on:keydown={onResizeInspectorWithKeyboard}></button>
      <slot name="inspector"></slot>
    </div>
    {/if}
  </section>
</section>
