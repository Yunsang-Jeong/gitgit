<script lang="ts">
  import { flip } from 'svelte/animate'
  import { cubicOut } from 'svelte/easing'
  import { onDestroy, onMount, tick } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { buildCommitGraph, defaultBranchGraphColorIndex, hasPrimaryBranchHead, isLocalPrimaryBranchHead, projectVisibleCommits } from '../lib/commit-graph'
  import { buildCommitGraphDrawing, commitGraphLaneColor, commitGraphRowHeight } from '../lib/commit-graph-render'
  import { commitGraphLaneLimitForWidth, commitGraphWidthForLaneCount, minimumVisibleGraphLanes } from '../lib/commit-graph-sizing'
  import { buildHistoryDateRows, buildHistoryRowGeometry } from '../lib/history-date-rows'
  import { isCommitVisible } from '../lib/history'
  import { directionalDropInsertionIndex, moveCommitTo, stabilizeDragPreview, type DragPreviewState } from '../lib/commit-edit'
  import { isLocalDefaultBranchBadge, summarizeRefBadges } from '../lib/remotes'
  import type { CommitFilterLogic, CommitFilterRule, CommitSummary, ContextMenuItem, HistoryFilterProgress, RemoteBadgeRule, RemoteInfo } from '../lib/types'

  export let commits: CommitSummary[] = []
  export let defaultBranch = ''
  export let allBranches = false
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
  export let editMode = false
  export let editCommits: CommitSummary[] = []
  export let editableCommitIDs: string[] = []
  export let editMovedCommitIDs: string[] = []
  export let onEditMove: (commitID: string, insertionIndex: number) => boolean = () => false

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
  let draggedEditCommit = ''
  let editDragPreview: DragPreviewState | null = null
  let suppressEditClickUntil = 0
  let editReordering = false
  let editReorderTimer: ReturnType<typeof setTimeout> | undefined
  let focusedCommitID = ''

  const editRowFlip = { duration: 170, easing: cubicOut }
  const editPreviewHoldDistance = commitGraphRowHeight * 0.125

  $: previewEditCommits = editMode && draggedEditCommit && editDragPreview !== null
    ? moveCommitTo(editCommits, draggedEditCommit, editDragPreview.insertionIndex)
    : editCommits
  $: tableCommits = editMode ? previewEditCommits : commits
  $: displayedCommits = editMode
    ? tableCommits
    : tableCommits.filter((commit) => isCommitVisible(commit, rules, logic))
  $: historyRows = buildHistoryDateRows(displayedCommits)
  $: historyRowGeometry = buildHistoryRowGeometry(historyRows, commitGraphRowHeight)
  $: tableAllBranches = allBranches
  $: fullGraphLayout = buildCommitGraph(tableCommits, defaultBranch, graphLaneLimit, tableAllBranches)
  $: visibleGraphCommits = projectVisibleCommits(tableCommits, new Set(displayedCommits.map((commit) => commit.commit)))
  $: visiblePrimaryBranch = hasPrimaryBranchHead(visibleGraphCommits, defaultBranch, tableAllBranches) ? defaultBranch : ''
  $: localDefaultGraphLane = Boolean(visiblePrimaryBranch) && isLocalPrimaryBranchHead(visibleGraphCommits, visiblePrimaryBranch, tableAllBranches)
  $: graphLayout = buildCommitGraph(visibleGraphCommits, visiblePrimaryBranch, graphLaneLimit, tableAllBranches)
  $: graphWidth = commitGraphWidthForLaneCount(graphLayout.laneCount)
  $: graphDrawing = buildCommitGraphDrawing(visibleGraphCommits, graphLayout.rows, Boolean(visiblePrimaryBranch), historyRowGeometry.tops, historyRowGeometry.height)
  $: graphVersion = `${graphLaneLimit}|${visiblePrimaryBranch}|${localDefaultGraphLane}|${historyRows.map((row) => `${row.separator?.key ?? ''}:${row.commit.commit}:${(row.commit.parents ?? []).join(',')}`).join(';')}`
  $: editableEditCommitIDSet = new Set(editableCommitIDs)
  $: activeRowIndex = resolveActiveRowIndex(historyRows, selectedCommit, focusedCommitID)

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
    if (editMode || loading || loadingMore || !hasMore || !autoLoad) return
    const target = event.currentTarget as HTMLDivElement
    if (target.scrollTop <= 0) return
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight
    if (remaining <= target.clientHeight / 2) onLoadMore()
  }

  function resolveActiveRowIndex(rows: typeof historyRows, selected: string, focused: string): number {
    const bySelected = rows.findIndex((row) => row.commit.commit === selected)
    if (bySelected >= 0) return bySelected
    const byFocus = rows.findIndex((row) => row.commit.commit === focused)
    return byFocus >= 0 ? byFocus : 0
  }

  function presentedCommitFor(commit: CommitSummary): CommitSummary {
    const historicalBranch = fullGraphLayout.historicalBranches.get(commit.commit) ?? ''
    return historicalBranch ? { ...commit, historical_branch: historicalBranch } : commit
  }

  function selectWithKeyboard(event: KeyboardEvent, index: number, commit: CommitSummary): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onSelect(commit)
      return
    }
    moveRowFocus(event, index)
  }

  function moveRowFocus(event: KeyboardEvent, index: number): void {
    const step = event.key === 'PageDown' || event.key === 'PageUp' ? 10 : 1
    const last = historyRows.length - 1
    let next = index
    if (event.key === 'ArrowDown' || event.key === 'PageDown') next = Math.min(last, index + step)
    else if (event.key === 'ArrowUp' || event.key === 'PageUp') next = Math.max(0, index - step)
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = last
    else return

    event.preventDefault()
    if (next === index) return
    const row = historyRows[next]
    focusedCommitID = row.commit.commit
    onSelect(presentedCommitFor(row.commit))
    void tick().then(() => focusRow(next))
  }

  function focusRow(index: number): void {
    const rows = commitTableElement?.querySelectorAll<HTMLElement>('.commit-row')
    const row = rows?.[index]
    if (!row) return
    row.focus({ preventScroll: true })
    row.scrollIntoView({ block: 'nearest' })
  }

  function beginEditDrag(event: DragEvent, commit: CommitSummary): void {
    if (!isEditableEditCommit(commit.commit)) return
    draggedEditCommit = commit.commit
    editDragPreview = null
    event.dataTransfer?.setData('text/plain', commit.commit)
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }

  function insertionIndexForEditDrop(event: DragEvent, targetCommitID: string, sourceCommitID = draggedEditCommit): number {
    const targetIndex = editCommits.findIndex((commit) => commit.commit === targetCommitID)
    if (targetIndex < 0) return editCommits.length
    const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const sourceIndex = editCommits.findIndex((commit) => commit.commit === sourceCommitID)
    return directionalDropInsertionIndex({
      sourceIndex,
      targetIndex,
      pointerY: event.clientY,
      targetTop: bounds.top,
      targetHeight: bounds.height,
    })
  }

  function updateEditDropTarget(event: DragEvent, targetCommitID: string): void {
    if (!editMode) return
    const commit = draggedEditCommit || event.dataTransfer?.getData('text/plain') || ''
    if (!commit || !isEditableEditCommit(commit) || !isEditableEditCommit(targetCommitID)) return
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    if (targetCommitID === commit && editDragPreview) return

    const insertionIndex = targetCommitID === commit
      ? null
      : insertionIndexForEditDrop(event, targetCommitID, commit)
    const candidateInsertionIndex = insertionIndex !== null && moveCommitTo(editCommits, commit, insertionIndex) !== editCommits
      ? insertionIndex
      : null
    const nextPreview = stabilizeDragPreview(editDragPreview, candidateInsertionIndex, event.clientY, editPreviewHoldDistance)
    if (nextPreview === editDragPreview) return

    const previewOrderChanged = nextPreview?.insertionIndex !== editDragPreview?.insertionIndex
    editDragPreview = nextPreview
    if (previewOrderChanged) beginEditReorderAnimation()
  }

  function commitEditDrop(event: DragEvent, targetCommitID: string): void {
    if (!editMode) return
    const commit = draggedEditCommit || event.dataTransfer?.getData('text/plain') || ''
    if (!commit || !isEditableEditCommit(commit) || !isEditableEditCommit(targetCommitID)) {
      clearEditDrag()
      return
    }
    event.preventDefault()
    const previewWasActive = editDragPreview !== null
    const insertionIndex = editDragPreview?.insertionIndex ?? insertionIndexForEditDrop(event, targetCommitID, commit)
    if (commit && onEditMove(commit, insertionIndex) && !previewWasActive) beginEditReorderAnimation()
    suppressEditClickUntil = Date.now() + 120
    editDragPreview = null
    draggedEditCommit = ''
  }

  function clearEditDrag(): void {
    editDragPreview = null
    draggedEditCommit = ''
  }

  function isEditableEditCommit(commitID: string): boolean {
    return editMode && editableEditCommitIDSet.has(commitID)
  }

  function beginEditReorderAnimation(): void {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    editReordering = true
    if (editReorderTimer) clearTimeout(editReorderTimer)
    editReorderTimer = setTimeout(() => {
      editReordering = false
      editReorderTimer = undefined
    }, editRowFlip.duration)
  }

  function selectDisplayedCommit(commit: CommitSummary): void {
    if (editMode && Date.now() < suppressEditClickUntil) return
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
    if (editReorderTimer) clearTimeout(editReorderTimer)
  })
</script>

<div bind:this={commitTableElement} class:edit-mode={editMode} class:edit-reordering={editReordering} class="commit-table" role="table" aria-label={editMode ? 'Editable commit history' : 'Commit history'} style={`--graph-width: ${graphWidth}px; --commit-row-height: ${commitGraphRowHeight}px`}>
  <div class="commit-table-scroll" role="presentation" on:scroll={handleScroll}>
    <div class="commit-body" role="rowgroup">
      {#if loading}
        <div class="history-empty"><span class="empty-spinner"></span><strong>Loading commit history…</strong></div>
      {:else if tableCommits.length === 0}
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
              <path class:local-default={localDefaultGraphLane && path.primary} class:overflow={path.overflow} class:primary={path.primary} d={path.d} stroke={path.gradientID ? `url(#${path.gradientID})` : path.color} />
            {/each}
            {#each graphDrawing.nodes as node (node.commit)}
              {#if node.primary}
                <circle class:local-default={localDefaultGraphLane} class:selected={node.commit === selectedCommit} class="graph-node-ring" cx={node.x} cy={node.y} r="6" />
              {/if}
              <circle class:local-default={localDefaultGraphLane && node.primary} class="graph-node" cx={node.x} cy={node.y} r={node.radius} fill={node.color} />
            {/each}
          </svg>
        {/key}
        {#key editMode}
          {#each historyRows as historyRow, editRowIndex (historyRow.commit.commit)}
            {@const commit = historyRow.commit}
            {@const historicalBranch = fullGraphLayout.historicalBranches.get(commit.commit) ?? ''}
            {@const historicalBranchTip = fullGraphLayout.historicalBranchTips.get(commit.commit) ?? ''}
            {@const presentedCommit = presentedCommitFor(commit)}
            {@const refSummary = summarizeRefBadges(commit.refs, remotes, remoteBadgeRules, showRemoteBadges, defaultBranch)}
            {@const primaryRef = refSummary.primary}
            {@const graphRow = graphLayout.rows.get(commit.commit)}
            {@const localDefaultBranch = isLocalDefaultBranchBadge(primaryRef, defaultBranch)}
            {@const editableEditCommit = isEditableEditCommit(commit.commit)}
            {@const refColor = localDefaultBranch
              ? '#e7f8ff'
              : graphRow?.nodeOverflow
                ? '#8d9da3'
                : commitGraphLaneColor(graphRow?.nodeColor ?? defaultBranchGraphColorIndex)}
            <div class="commit-row-motion" role="presentation" animate:flip={editRowFlip}>
              {#if historyRow.separator}
                <div class="history-date-separator" role="row" aria-label={historyRow.separator.label}>
                  <span role="cell"><time>{historyRow.separator.label}</time></span><i aria-hidden="true"></i>
                </div>
              {/if}
              <div
                class:selected={selectedCommit === commit.commit}
                class:edit-mode-row={editMode}
                class:edit-readonly-row={editMode && !editableEditCommit}
                class:moved={editMode && editMovedCommitIDs.includes(commit.commit)}
                class:dragging={editableEditCommit && draggedEditCommit === commit.commit}
                class="commit-row commit-grid"
                role="row"
                tabindex={editRowIndex === activeRowIndex ? 0 : -1}
                aria-selected={selectedCommit === commit.commit}
                draggable={editableEditCommit}
                on:click={() => selectDisplayedCommit(presentedCommit)}
                on:contextmenu|preventDefault={(event) => openMessageMenu(event, presentedCommit)}
                on:focus={() => (focusedCommitID = commit.commit)}
                on:keydown={(event) => selectWithKeyboard(event, editRowIndex, presentedCommit)}
                on:dragstart={(event) => beginEditDrag(event, presentedCommit)}
                on:dragover={(event) => updateEditDropTarget(event, commit.commit)}
                on:drop={(event) => commitEditDrop(event, commit.commit)}
                on:dragend={clearEditDrag}
              >
                <span class="history-ref-cell" role="cell">
                  {#if primaryRef}
                    <small class:local-default={localDefaultBranch} class:remote={primaryRef.remote} class="history-primary-ref" style={`--history-ref-color: ${refColor}`} title={primaryRef.title} aria-label={primaryRef.label}>
                      {#if primaryRef.remote}<RemoteBadgeIcon name={primaryRef.icon} />{/if}
                      <span>{primaryRef.branch}</span>
                    </small>
                    {#if refSummary.remaining.length}
                      <small class="history-ref-count" title={`Additional refs: ${refSummary.remaining.map((badge) => badge.title).join(', ')}`} aria-label={`${refSummary.remaining.length} additional refs`}>+{refSummary.remaining.length}</small>
                    {/if}
                  {:else if historicalBranchTip}
                    <small class="history-primary-ref history-branch-tip" style={`--history-ref-color: ${refColor}`} title={`Merged branch: ${historicalBranchTip}`} aria-label={`Merged branch ${historicalBranchTip}`}>
                      <span>{historicalBranchTip}</span>
                    </small>
                  {/if}
                </span>
                <span class="graph-cell" role="cell" aria-label={`${(commit.parents ?? []).length} parents`}>
                </span>
                <span class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'message'} class="history-data-cell history-message-cell context-action" role="cell" title={`${commit.message}\n\nSelect commit · Right-click for actions`}>
                  <strong class="copy-target">{title(commit.message)}</strong>
                  {#if commit.commit === branchPoint}
                    <small class="branch-point" title="Common ancestor with the default branch">Branch point</small>
                  {/if}
                  <span class="history-author" title={commit.author.email ? `${commit.author.name} <${commit.author.email}>` : commit.author.name}>{commit.author.name || commit.author.email}</span>
                </span>
              </div>
            </div>
          {/each}
        {/key}
        {#if !editMode && filterProgress}
          <div class="history-filter-progress" role="status" aria-live="polite">
            <span class="empty-spinner"></span>
            <span class="history-filter-progress-copy">
              <strong>Searching more history…</strong>
              <small>Preset · {filterProgress.presets.join(', ')} · {filterProgress.conditions.join(' · ')}</small>
              <small>Targeting about {filterProgress.target} visible commits · {filterProgress.visible} found · checked {filterProgress.scanned} of {filterProgress.total} · scanning {filterProgress.scope} toward its initial commit</small>
            </span>
          </div>
        {:else if !editMode && hasMore}
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
