<script lang="ts">
  import { onDestroy } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { formatDate } from '../lib/datetime'
  import { isCommitHighlighted, isCommitVisible } from '../lib/history'
  import { visibleRefBadges } from '../lib/remotes'
  import type { CommitFilterAction, CommitFilterLogic, CommitFilterRule, CommitSummary, ContextMenuItem, RemoteBadgeRule, RemoteInfo } from '../lib/types'

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
  export let hasMore = false
  export let autoLoad = true
  export let branchPoint = ''
  export let onSelect: (commit: CommitSummary) => void
  export let onLoadMore: () => void
  export let onAddFilter: (action: CommitFilterAction, field: 'author' | 'message', pattern: string) => void
  export let onSearchMessage: (message: string) => void
  export let onUseSearchAuthor: (author: string) => void

  type GraphEdge = {
    from: number
    to: number
  }

  type GraphRow = {
    lane: number
    passThrough: number[]
    incoming: GraphEdge[]
    outgoing: GraphEdge[]
  }

  type GraphLayout = {
    rows: Map<string, GraphRow>
    laneCount: number
  }

  const laneColors = ['#55a7e8', '#d568a7', '#62b97a', '#d69a2d', '#9a78d1']

  let contextMenu: {
    x: number
    y: number
    label: string
    items: ContextMenuItem[]
    target: { commit: string; field: 'commit' | 'message' | 'author' | 'date' }
  } | null = null
  let copyToast = ''
  let copyToastTimer: ReturnType<typeof setTimeout> | undefined

  $: graphLayout = buildGraph(commits, defaultBranch)
  $: graphRows = graphLayout.rows
  $: displayedCommits = commits.filter((commit) => isCommitVisible(commit, rules, logic))
  $: graphWidth = Math.max(76, Math.min(144, 28 + Math.max(0, graphLayout.laneCount - 1) * 13))
  $: laneSpacing = graphLayout.laneCount <= 1 ? 13 : Math.min(13, (graphWidth - 28) / (graphLayout.laneCount - 1))

  function title(message: string): string {
    return message.split('\n')[0]
  }

  function buildGraph(items: CommitSummary[], primaryBranch: string): GraphLayout {
    const commitsByID = new Map(items.map((commit) => [commit.commit, commit]))
    const defaultChain = new Set<string>()
    const defaultHead = primaryBranch
      ? items.find((commit) => commit.refs?.includes(primaryBranch))
      : undefined

    let defaultCommit = defaultHead
    while (defaultCommit && !defaultChain.has(defaultCommit.commit)) {
      defaultChain.add(defaultCommit.commit)
      defaultCommit = commitsByID.get(defaultCommit.parents?.[0] ?? '')
    }

    const reserveDefaultLane = Boolean(primaryBranch)
    const lanes: Array<string | null> = reserveDefaultLane ? [null] : []
    const rows = new Map<string, GraphRow>()
    let laneCount = lanes.length

    for (const commit of items) {
      const onDefaultChain = defaultChain.has(commit.commit)
      let incomingLanes = lanes
        .map((value, index) => value === commit.commit ? index : -1)
        .filter((index) => index >= 0)

      if (onDefaultChain && !incomingLanes.includes(0)) {
        lanes[0] = commit.commit
        incomingLanes = [0, ...incomingLanes]
      } else if (incomingLanes.length === 0) {
        const start = reserveDefaultLane ? 1 : 0
        const available = lanes.findIndex((value, index) => index >= start && value === null)
        const lane = available >= 0 ? available : lanes.length
        lanes[lane] = commit.commit
        incomingLanes = [lane]
      }

      const lane = onDefaultChain ? 0 : incomingLanes[0]
      const passThrough = lanes
        .map((value, index) => value !== null && !incomingLanes.includes(index) ? index : -1)
        .filter((index) => index >= 0)
      const incoming = incomingLanes.map((from) => ({ from, to: lane }))

      for (const index of incomingLanes) lanes[index] = null

      const outgoing: GraphEdge[] = []
      for (const [parentIndex, parent] of (commit.parents ?? []).entries()) {
        let target = lanes.indexOf(parent)
        if (target < 0) {
          if (parentIndex === 0 && lanes[lane] === null) {
            target = lane
          } else {
            const start = reserveDefaultLane ? 1 : 0
            const available = lanes.findIndex((value, index) => index >= start && value === null)
            target = available >= 0 ? available : lanes.length
          }
          lanes[target] = parent
        }
        if (!outgoing.some((edge) => edge.to === target)) outgoing.push({ from: lane, to: target })
      }

      while (lanes.length > (reserveDefaultLane ? 1 : 0) && lanes[lanes.length - 1] === null) lanes.pop()
      laneCount = Math.max(laneCount, lane + 1, ...passThrough.map((value) => value + 1), ...outgoing.map((edge) => edge.to + 1))
      rows.set(commit.commit, { lane, passThrough, incoming, outgoing })
    }
    return { rows, laneCount: Math.max(1, laneCount) }
  }

  function laneX(lane: number): number {
    return 14 + lane * laneSpacing
  }

  function laneColor(lane: number): string {
    return laneColors[lane % laneColors.length]
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

  function filterItems(field: 'author' | 'message', pattern: string): ContextMenuItem[] {
    return (['hide', 'show', 'highlight'] as CommitFilterAction[]).map((action, index) => ({
      label: `Add filter: ${action[0].toLocaleUpperCase()}${action.slice(1)}`,
      separatorBefore: index === 0,
      run: () => onAddFilter(action, field, pattern),
    }))
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
        ...filterItems('author', author),
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
        ...filterItems('message', message),
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

<div class="commit-table" role="table" aria-label="Commit history" style={`--graph-width: ${graphWidth}px`}>
  <div class="commit-table-scroll" on:scroll={handleScroll}>
    <div class="commit-header commit-grid" role="row">
      <span title={`Topology of the loaded commits${defaultBranch ? `; ${defaultBranch} stays on the left` : ''}`}>Graph</span>
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
        <div class="history-empty"><span class="empty-symbol">∅</span><strong>No commits match active filters</strong></div>
      {:else}
        {#each displayedCommits as commit}
            {@const graph = graphRows.get(commit.commit)}
            <div
              class:selected={selectedCommit === commit.commit}
              class:highlighted={isCommitHighlighted(commit, rules)}
              class="commit-row commit-grid"
              role="row"
              tabindex="0"
              aria-selected={selectedCommit === commit.commit}
              on:click={() => onSelect(commit)}
              on:keydown={(event) => selectWithKeyboard(event, commit)}
            >
              <span class="graph-cell" role="cell" aria-label={`${(commit.parents ?? []).length} parents`}>
                {#if graph}
                  <svg viewBox={`0 0 ${graphWidth} 38`} aria-hidden="true">
                    {#each graph.passThrough as lane}
                      <line x1={laneX(lane)} y1="0" x2={laneX(lane)} y2="38" stroke={laneColor(lane)} />
                    {/each}
                    {#each graph.incoming as edge}
                      {#if edge.from === edge.to}
                        <line x1={laneX(edge.from)} y1="0" x2={laneX(edge.to)} y2="19" stroke={laneColor(edge.from)} />
                      {:else}
                        <path d={`M ${laneX(edge.from)} 0 C ${laneX(edge.from)} 9, ${laneX(edge.to)} 10, ${laneX(edge.to)} 19`} stroke={laneColor(edge.from)} />
                      {/if}
                    {/each}
                    {#each graph.outgoing as edge}
                      {#if edge.from === edge.to}
                        <line x1={laneX(edge.from)} y1="19" x2={laneX(edge.to)} y2="38" stroke={laneColor(edge.to)} />
                      {:else}
                        <path d={`M ${laneX(edge.from)} 19 C ${laneX(edge.from)} 28, ${laneX(edge.to)} 29, ${laneX(edge.to)} 38`} stroke={laneColor(edge.to)} />
                      {/if}
                    {/each}
                    {#if graph.lane === 0 && defaultBranch}
                      <circle class="graph-node-ring" cx={laneX(graph.lane)} cy="19" r="6.5" />
                    {/if}
                    <circle class="graph-node" cx={laneX(graph.lane)} cy="19" r={graph.lane === 0 && defaultBranch ? 4.5 : 4} fill={laneColor(graph.lane)} />
                  </svg>
                {/if}
              </span>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'commit'} class="history-data-cell history-commit-cell" type="button" role="cell" title="Select commit · Right-click for copy" on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openCommitMenu(event, commit)}><code class="copy-target">{commit.short_commit}</code></button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'message'} class="history-data-cell history-message-cell" type="button" role="cell" title={`${commit.message}\n\nSelect commit · Right-click for actions`} on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openMessageMenu(event, commit)}>
                <strong class="copy-target">{title(commit.message)}</strong>
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
              </button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'author'} class="history-data-cell" type="button" role="cell" title={`${commit.author.name} <${commit.author.email}>\n\nSelect commit · Right-click for actions`} on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openAuthorMenu(event, commit)}><span class="copy-target">{commit.author.name}</span></button>
              <button class:interaction-active={contextMenu?.target.commit === commit.commit && contextMenu.target.field === 'date'} class="history-data-cell" type="button" role="cell" title="Select commit · Right-click for copy" on:click|stopPropagation={() => onSelect(commit)} on:contextmenu|preventDefault|stopPropagation={(event) => openDateMenu(event, commit)}><span class="copy-target">{formatDate(commit.date)}</span></button>
            </div>
        {/each}
        {#if hasMore}
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
