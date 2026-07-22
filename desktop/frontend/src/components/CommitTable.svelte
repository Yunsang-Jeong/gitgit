<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { buildCommitGraph, defaultBranchGraphColorIndex, hasPrimaryBranchHead, projectVisibleCommits } from '../lib/commit-graph'
  import { buildCommitGraphDrawing, commitGraphLaneColor, commitGraphRowHeight } from '../lib/commit-graph-render'
  import { commitGraphLaneLimitForWidth, commitGraphWidthForLaneCount, minimumVisibleGraphLanes } from '../lib/commit-graph-sizing'
  import { buildHistoryDateRows, buildHistoryRowGeometry } from '../lib/history-date-rows'
  import { isCommitVisible } from '../lib/history'
  import { summarizeRefBadges } from '../lib/remotes'
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

  let contextMenu: {
    x: number
    y: number
    label: string
    items: ContextMenuItem[]
    target: { commit: string; field: 'message' }
  } | null = null
  let copyToast = ''
  let copyToastTimer: ReturnType<typeof setTimeout> | undefined
  let commitTableElement: HTMLDivElement
  let graphLaneLimit = minimumVisibleGraphLanes

  $: displayedCommits = commits.filter((commit) => isCommitVisible(commit, rules, logic))
  $: historyRows = buildHistoryDateRows(displayedCommits)
  $: historyRowGeometry = buildHistoryRowGeometry(historyRows, commitGraphRowHeight)
  $: fullGraphLayout = buildCommitGraph(commits, defaultBranch, graphLaneLimit)
  $: visibleGraphCommits = projectVisibleCommits(commits, new Set(displayedCommits.map((commit) => commit.commit)))
  $: visiblePrimaryBranch = hasPrimaryBranchHead(visibleGraphCommits, defaultBranch) ? defaultBranch : ''
  $: graphLayout = buildCommitGraph(visibleGraphCommits, visiblePrimaryBranch, graphLaneLimit)
  $: graphWidth = commitGraphWidthForLaneCount(graphLayout.laneCount)
  $: graphDrawing = buildCommitGraphDrawing(visibleGraphCommits, graphLayout.rows, Boolean(visiblePrimaryBranch), historyRowGeometry.tops, historyRowGeometry.height)
  $: graphVersion = `${graphLaneLimit}|${visiblePrimaryBranch}|${historyRows.map((row) => `${row.separator?.key ?? ''}:${row.commit.commit}:${(row.commit.parents ?? []).join(',')}`).join(';')}`

  onMount(() => {
    const observer = new ResizeObserver(([entry]) => {
      const nextLimit = commitGraphLaneLimitForWidth(entry.contentRect.width)
      if (nextLimit !== graphLaneLimit) graphLaneLimit = nextLimit
    })
    observer.observe(commitTableElement)
    return () => observer.disconnect()
  })

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

  onDestroy(() => {
    if (copyToastTimer) clearTimeout(copyToastTimer)
  })
</script>

<div bind:this={commitTableElement} class="commit-table" role="table" aria-label="Commit history" style={`--graph-width: ${graphWidth}px; --commit-row-height: ${commitGraphRowHeight}px`}>
  <div class="commit-table-scroll" on:scroll={handleScroll}>
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
        {#key graphVersion}
          <svg class="commit-graph-overlay" viewBox={`0 0 ${graphWidth} ${graphDrawing.height}`} width={graphWidth} height={graphDrawing.height} aria-hidden="true">
            <defs>
              {#each graphDrawing.gradients as gradient (gradient.id)}
                <linearGradient id={gradient.id} gradientUnits="userSpaceOnUse" x1={gradient.x1} y1={gradient.y1} x2={gradient.x2} y2={gradient.y2}>
                  <stop offset="0%" stop-color={gradient.startColor} />
                  <stop offset="100%" stop-color={gradient.endColor} />
                </linearGradient>
              {/each}
            </defs>
            {#each graphDrawing.paths as path, index (path.gradientID ?? `${path.color}:${path.overflow}:${index}`)}
              <path class:overflow={path.overflow} d={path.d} stroke={path.gradientID ? `url(#${path.gradientID})` : path.color} />
            {/each}
            {#each graphDrawing.nodes as node (node.commit)}
              {#if node.primary}
                <circle class:selected={node.commit === selectedCommit} class="graph-node-ring" cx={node.x} cy={node.y} r="6" />
              {/if}
              <circle class="graph-node" cx={node.x} cy={node.y} r={node.radius} fill={node.color} />
            {/each}
          </svg>
        {/key}
        {#each historyRows as historyRow (historyRow.commit.commit)}
            {@const commit = historyRow.commit}
            {#if historyRow.separator}
              <div class="history-date-separator" role="separator" aria-label={historyRow.separator.label}>
                <time>{historyRow.separator.label}</time><span></span>
              </div>
            {/if}
            {@const historicalBranch = fullGraphLayout.historicalBranches.get(commit.commit) ?? ''}
            {@const presentedCommit = historicalBranch ? { ...commit, historical_branch: historicalBranch } : commit}
            {@const refSummary = summarizeRefBadges(commit.refs, remotes, remoteBadgeRules, showRemoteBadges, defaultBranch)}
            {@const primaryRef = refSummary.primary}
            {@const graphRow = graphLayout.rows.get(commit.commit)}
            {@const refColor = graphRow?.nodeOverflow ? '#8d9da3' : commitGraphLaneColor(graphRow?.nodeColor ?? defaultBranchGraphColorIndex)}
            <div
              class:selected={selectedCommit === commit.commit}
              class="commit-row commit-grid"
              role="row"
              tabindex="0"
              aria-selected={selectedCommit === commit.commit}
              on:click={() => onSelect(presentedCommit)}
              on:keydown={(event) => selectWithKeyboard(event, presentedCommit)}
            >
              <span class="history-ref-cell" role="cell">
                {#if primaryRef}
                  <small class:remote={primaryRef.remote} class="history-primary-ref" style={`--history-ref-color: ${refColor}`} title={primaryRef.title} aria-label={primaryRef.label}>
                    {#if primaryRef.remote}<RemoteBadgeIcon name={primaryRef.icon} />{/if}
                    <span>{primaryRef.branch}</span>
                  </small>
                  {#if refSummary.remaining.length}
                    <small class="history-ref-count" title={`Additional refs: ${refSummary.remaining.map((badge) => badge.title).join(', ')}`} aria-label={`${refSummary.remaining.length} additional refs`}>+{refSummary.remaining.length}</small>
                  {/if}
                {/if}
              </span>
              <span class="graph-cell" role="cell" aria-label={`${(commit.parents ?? []).length} parents`}>
              </span>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'message'} class="history-data-cell history-message-cell context-action" type="button" role="cell" title={`${commit.message}\n\nSelect commit · Right-click for actions`} on:click|stopPropagation={() => onSelect(presentedCommit)} on:contextmenu|preventDefault|stopPropagation={(event) => openMessageMenu(event, presentedCommit)}>
                <strong class="copy-target">{title(commit.message)}</strong>
                {#if commit.commit === branchPoint}
                  <small class="branch-point" title="Common ancestor with the default branch">Branch point</small>
                {/if}
              </button>
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
