<script lang="ts">
  import { onDestroy } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { formatDate } from '../lib/datetime'
  import { isCommitVisible } from '../lib/history'
  import { visibleRefBadges } from '../lib/remotes'
  import type { CommitFilterLogic, CommitFilterRule, CommitSummary, ContextMenuItem, HistoryFilterProgress, RemoteBadgeRule, RemoteInfo } from '../lib/types'

  export let commits: CommitSummary[] = []
  export let defaultBranch = ''
  export let remotes: RemoteInfo[] = []
  export let remoteBadgeRules: RemoteBadgeRule[] = []
  export let showRemoteBadges = false
  export let rules: CommitFilterRule[] = []
  export let logic: CommitFilterLogic
  export let selectedCommit = ''
  export let loading = false
  export let loadingMore = false
  export let filterProgress: HistoryFilterProgress | null = null
  export let hasMore = false
  export let autoLoad = true
  export let branchPoint = ''
  export let onSelect: (commit: CommitSummary) => void
  export let onLoadMore: () => void
  export let onSearchMessage: (message: string) => void
  export let onUseSearchAuthor: (author: string) => void

  let contextMenu: {
    x: number
    y: number
    label: string
    items: ContextMenuItem[]
    target: { commit: string; field: 'commit' | 'message' | 'author' | 'date' }
  } | null = null
  let copyToast = ''
  let copyToastTimer: ReturnType<typeof setTimeout> | undefined

  $: displayedCommits = commits.filter((commit) => isCommitVisible(commit, rules, logic))

  function title(message: string): string {
    return message.split('\n')[0]
  }

  function handleScroll(event: Event): void {
    if (loading || loadingMore || !hasMore || !autoLoad) return
    const target = event.currentTarget as HTMLDivElement
    if (target.scrollTop <= 0) return
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight
    if (remaining <= target.clientHeight / 2) onLoadMore()
  }

  function selectWithKeyboard(event: KeyboardEvent, commit: CommitSummary): void {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    onSelect(commit)
  }

  async function copyCell(value: string, label: string, commit: CommitSummary): Promise<void> {
    onSelect(commit)
    if (copyToastTimer) clearTimeout(copyToastTimer)
    try {
      await navigator.clipboard.writeText(value)
      copyToast = `${label} copied`
    } catch {
      copyToast = `Failed to copy ${label.toLocaleLowerCase()}`
    }
    copyToastTimer = setTimeout(() => (copyToast = ''), 1600)
  }

  function openAuthorMenu(event: MouseEvent, commit: CommitSummary): void {
    const author = commit.author.name || commit.author.email
    onSelect(commit)
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: `Actions for ${author}`,
      target: { commit: commit.commit, field: 'author' },
      items: [
        { label: 'Copy author', run: () => copyCell(`${commit.author.name} <${commit.author.email}>`, 'Author', commit) },
        { label: 'Use as Search author', separatorBefore: true, run: () => onUseSearchAuthor(author) },
      ],
    }
  }

  function openCommitMenu(event: MouseEvent, commit: CommitSummary): void {
    onSelect(commit)
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: 'Commit actions',
      target: { commit: commit.commit, field: 'commit' },
      items: [
        { label: 'Copy commit hash', run: () => copyCell(commit.commit, 'Commit hash', commit) },
      ],
    }
  }

  function openMessageMenu(event: MouseEvent, commit: CommitSummary): void {
    const message = title(commit.message)
    onSelect(commit)
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: 'Commit message actions',
      target: { commit: commit.commit, field: 'message' },
      items: [
        { label: 'Copy message', run: () => copyCell(commit.message, 'Message', commit) },
        { label: 'Add search', separatorBefore: true, run: () => onSearchMessage(message) },
      ],
    }
  }

  function openDateMenu(event: MouseEvent, commit: CommitSummary): void {
    const date = formatDate(commit.date)
    onSelect(commit)
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: 'Commit date actions',
      target: { commit: commit.commit, field: 'date' },
      items: [
        { label: 'Copy date', run: () => copyCell(date, 'Date', commit) },
      ],
    }
  }

  onDestroy(() => {
    if (copyToastTimer) clearTimeout(copyToastTimer)
  })
</script>

<div class="commit-table" role="table" aria-label="Commit history">
  <div class="commit-table-scroll" on:scroll={handleScroll}>
    <div class="commit-header commit-grid" role="row">
      <span>Commit</span>
      <span>Message</span>
      <span>Author</span>
      <span>Date</span>
    </div>
    <div class="commit-body">
      {#if loading}
        <div class="history-empty"><span class="empty-spinner"></span><strong>Loading commit history…</strong></div>
      {:else if commits.length === 0}
        <div class="history-empty"><span class="empty-symbol">∅</span><strong>No commits in this scope</strong></div>
      {:else if displayedCommits.length === 0}
        <div class="history-empty">
          {#if filterProgress}
            <span class="empty-spinner"></span><strong>Searching more history…</strong>
            <span>Preset · {filterProgress.presets.join(', ')}</span>
            <small>{filterProgress.conditions.join(' · ')}</small>
            <small>Targeting about {filterProgress.target} visible commits · {filterProgress.visible} found · checked {filterProgress.scanned} of {filterProgress.total} · scanning {filterProgress.scope} toward its initial commit</small>
          {:else}<span class="empty-symbol">∅</span><strong>No commits match active presets</strong>{/if}
        </div>
      {:else}
        {#each displayedCommits as commit}
            <div
              class:selected={selectedCommit === commit.commit}
              class="commit-row commit-grid"
              role="row"
              tabindex="0"
              aria-selected={selectedCommit === commit.commit}
              on:click={() => onSelect(commit)}
              on:keydown={(event) => selectWithKeyboard(event, commit)}
            >
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'commit'} class="history-data-cell history-commit-cell context-action" type="button" role="cell" title="Select commit · Right-click for copy" on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openCommitMenu(event, commit)}><code class="copy-target">{commit.short_commit}</code></button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'message'} class="history-data-cell history-message-cell context-action" type="button" role="cell" title={`${commit.message}\n\nSelect commit · Right-click for actions`} on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openMessageMenu(event, commit)}>
                <strong class="copy-target">{title(commit.message)}</strong>
                <span class="history-message-meta">
                  {#if commit.refs?.length}
                    <span class="history-ref-badges">
                      {#each visibleRefBadges(commit.refs, remotes, remoteBadgeRules, showRemoteBadges, defaultBranch) as badge (badge.ref)}
                        <small class:remote={badge.remote} title={badge.title} aria-label={badge.label}>
                          {#if badge.remote}<RemoteBadgeIcon name={badge.icon} />{/if}
                          <span>{badge.branch}</span>
                        </small>
                      {/each}
                    </span>
                  {/if}
                  {#if commit.commit === branchPoint}
                    <small class="branch-point" title="Common ancestor with the default branch">Branch point</small>
                  {/if}
                </span>
              </button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'author'} class="history-data-cell context-action" type="button" role="cell" title={`${commit.author.name} <${commit.author.email}>\n\nSelect commit · Right-click for actions`} on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openAuthorMenu(event, commit)}><span class="copy-target">{commit.author.name}</span></button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'date'} class="history-data-cell context-action" type="button" role="cell" title="Select commit · Right-click for copy" on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openDateMenu(event, commit)}><span class="copy-target">{formatDate(commit.date)}</span></button>
            </div>
        {/each}
        {#if filterProgress}
          <div class="history-filter-progress" role="status" aria-live="polite">
            <span class="empty-spinner"></span>
            <span class="history-filter-progress-copy">
              <strong>Searching more history…</strong>
              <small>Preset · {filterProgress.presets.join(', ')} · {filterProgress.conditions.join(' · ')}</small>
              <small>Targeting about {filterProgress.target} visible commits · {filterProgress.visible} found · checked {filterProgress.scanned} of {filterProgress.total} · scanning {filterProgress.scope} toward its initial commit</small>
            </span>
          </div>
        {:else if hasMore}
          <button class="history-load-more" type="button" disabled={loadingMore} on:click={onLoadMore}>
            {#if loadingMore}<span class="empty-spinner"></span>{/if}
            <span>{loadingMore ? 'Loading more commits…' : !autoLoad && branchPoint ? 'Load history before branch point' : 'Load more commits'}</span>
          </button>
        {/if}
      {/if}
    </div>
  </div>
</div>
{#if copyToast}<div class:failed={copyToast.startsWith('Failed')} class="table-copy-toast" role="status" aria-live="polite">{copyToast}</div>{/if}
{#if contextMenu}
  <ContextMenu x={contextMenu.x} y={contextMenu.y} items={contextMenu.items} ariaLabel={contextMenu.label} onClose={() => (contextMenu = null)} />
{/if}
