<script lang="ts">
  import { onDestroy } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import RepositoryTree from './RepositoryTree.svelte'
  import { addedAndDeletedLines, buildChangedFileTree, type ChangedFileTreeNode } from '../lib/changed-files'
  import { formatDate } from '../lib/datetime'
  import { buildReviewLink } from '../lib/review-links'
  import { defaultFirstRefs } from '../lib/remotes'
  import type { ChangedFilesView, CommitDetail, CommitFilterAction, ContextMenuItem, FileChange, RemoteInfo, RepositoryTreeResponse, SearchResult } from '../lib/types'

  export let selected: CommitDetail | SearchResult | null
  export let fileRevision: string
  export let allUsesDefault: boolean
  export let defaultChangedFilesView: ChangedFilesView = 'list'
  export let remotes: RemoteInfo[] = []
  export let defaultBranch = ''
  export let upstream = ''
  export let onOpenIDE: (path: string) => void
  export let onOpenFinder: (path: string) => void
  export let onOpenTerminal: (path: string) => void
  export let onOpenExternalURL: (url: string) => void
  export let onSelectFile: (path: string) => Promise<void>
  export let onAddFilter: (action: CommitFilterAction, path: string) => void
  export let onAddFileSearch: (path: string) => void
  export let onLoadTree: (revision: string, directory: string) => Promise<RepositoryTreeResponse>

  type InspectorTab = 'changes' | 'files'
  type VisibleChangedNode = { node: ChangedFileTreeNode; depth: number }
  type PopoverPosition = { top: number; left: number; width: number; maxHeight: number; arrowTop: number }

  let activeTab: InspectorTab = 'changes'
  let changedFilesView = defaultChangedFilesView
  let observedDefaultChangedFilesView = defaultChangedFilesView
  let observedCommit = selected?.commit ?? ''
  let collapsedDirectories = new Set<string>()
  let diffFile: FileChange | null = null
  let diffLoading = false
  let diffRequestID = 0
  let popoverPosition: PopoverPosition | null = null
  let copyToast = ''
  let copyToastTimer: ReturnType<typeof setTimeout> | undefined
  let contextMenu: { x: number; y: number; label: string; items: ContextMenuItem[]; targetPath: string } | null = null

  $: changedFileTree = buildChangedFileTree(selected?.files ?? [])
  $: selectedParents = selected && 'parents' in selected ? selected.parents : []
  $: selectedRefs = defaultFirstRefs(selected?.refs, defaultBranch)
  $: isMergeCommit = selectedParents.length > 1
  $: reviewLink = selected ? buildReviewLink(selected.message, selectedParents, remotes, upstream) : null
  $: visibleChangedNodes = flattenChangedFileTree(changedFileTree)
  $: diffLines = diffFile && selected?.file.path === diffFile.path ? addedAndDeletedLines(selected.diff) : []
  $: if (defaultChangedFilesView !== observedDefaultChangedFilesView) {
    observedDefaultChangedFilesView = defaultChangedFilesView
    changedFilesView = defaultChangedFilesView
    closeDiffPopover()
  }
  $: if ((selected?.commit ?? '') !== observedCommit) {
    observedCommit = selected?.commit ?? ''
    collapsedDirectories = new Set<string>()
    closeDiffPopover()
  }

  function title(message: string): string {
    return message.split('\n')[0]
  }

  function fileLabel(file: FileChange): string {
    return file.old_path ? `${file.old_path} → ${file.path}` : file.path
  }

  function flattenChangedFileTree(nodes: ChangedFileTreeNode[], depth = 0): VisibleChangedNode[] {
    const visible: VisibleChangedNode[] = []
    for (const node of nodes) {
      visible.push({ node, depth })
      if (node.kind === 'directory' && !collapsedDirectories.has(node.path)) {
        visible.push(...flattenChangedFileTree(node.children, depth + 1))
      }
    }
    return visible
  }

  function toggleChangedDirectory(path: string): void {
    const next = new Set(collapsedDirectories)
    if (next.has(path)) next.delete(path)
    else next.add(path)
    collapsedDirectories = next
    closeDiffPopover()
  }

  function setChangedFilesView(view: ChangedFilesView): void {
    changedFilesView = view
    closeDiffPopover()
  }

  function setActiveTab(tab: InspectorTab): void {
    activeTab = tab
    closeDiffPopover()
  }

  async function copyLayer(value: string, label: string): Promise<void> {
    if (!value) return
    if (copyToastTimer) clearTimeout(copyToastTimer)
    try {
      await navigator.clipboard.writeText(value)
      copyToast = `Success to copy · ${label}`
    } catch {
      copyToast = `Failed to copy · ${label}`
    }
    copyToastTimer = setTimeout(() => (copyToast = ''), 1800)
  }

  async function openDiffPopover(event: MouseEvent, file: FileChange): Promise<void> {
    event.stopPropagation()
    contextMenu = null
    diffFile = file
    diffLoading = true
    popoverPosition = positionDiffPopover((event.currentTarget as HTMLElement).getBoundingClientRect())
    const requestID = ++diffRequestID
    try {
      await onSelectFile(file.path)
    } finally {
      if (requestID === diffRequestID) diffLoading = false
    }
  }

  function positionDiffPopover(anchor: DOMRect): PopoverPosition {
    const gap = 14
    const viewportPadding = 16
    const availableWidth = Math.max(320, anchor.left - gap - viewportPadding)
    const width = Math.min(720, Math.max(360, window.innerWidth * 0.42), availableWidth)
    const maxHeight = Math.min(540, window.innerHeight - viewportPadding * 2)
    const left = Math.max(viewportPadding, anchor.left - gap - width)
    const top = Math.min(Math.max(viewportPadding, anchor.top - 42), window.innerHeight - maxHeight - viewportPadding)
    const arrowTop = Math.min(Math.max(18, anchor.top + anchor.height / 2 - top - 9), maxHeight - 28)
    return { top, left, width, maxHeight, arrowTop }
  }

  function closeDiffPopover(): void {
    diffRequestID += 1
    diffFile = null
    diffLoading = false
    popoverPosition = null
  }

  function handleWindowKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape' && diffFile) closeDiffPopover()
  }

  function openPathContextMenu(event: MouseEvent, path: string, pattern = path): void {
    const filterItems = (['hide', 'show', 'highlight'] as CommitFilterAction[]).map((action, index) => ({
      label: `Add filter: ${action[0].toLocaleUpperCase()}${action.slice(1)}`,
      separatorBefore: index === 0,
      run: () => onAddFilter(action, pattern),
    }))
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: `Actions for ${path}`,
      targetPath: path,
      items: [
        { label: 'Copy path', run: () => copyLayer(path, 'File path') },
        ...filterItems,
        { label: 'Add search', separatorBefore: true, run: () => onAddFileSearch(pattern) },
        { label: 'Open Finder', separatorBefore: true, run: () => onOpenFinder(path) },
        { label: 'Open Terminal', run: () => onOpenTerminal(path) },
      ],
    }
  }

  onDestroy(() => {
    if (copyToastTimer) clearTimeout(copyToastTimer)
  })
</script>

<svelte:window on:keydown={handleWindowKeydown} on:mousedown={() => closeDiffPopover()} on:resize={() => closeDiffPopover()} />

<aside class="inspector pane">
  <div class="pane-title inspector-pane-title">
    <span>Inspector</span>
    <div class="inspector-tabs" role="tablist" aria-label="Inspector views">
      <button
        class:active={activeTab === 'changes'}
        type="button"
        role="tab"
        aria-selected={activeTab === 'changes'}
        on:click={() => setActiveTab('changes')}
      >Changes</button>
      <button
        class:active={activeTab === 'files'}
        type="button"
        role="tab"
        aria-selected={activeTab === 'files'}
        on:click={() => setActiveTab('files')}
      >Files</button>
    </div>
  </div>
  {#if activeTab === 'files'}
    <RepositoryTree
      revision={fileRevision}
      {allUsesDefault}
      changedFiles={selected?.files ?? []}
      {onLoadTree}
      onCopyPath={(path) => void copyLayer(path, 'File path')}
      {onAddFilter}
      onAddSearch={onAddFileSearch}
      {onOpenFinder}
      {onOpenTerminal}
    />
  {:else if !selected}
    <div class="inspector-empty">
      <span>⌘</span>
      <strong>Select a commit</strong>
      <p>Commit metadata and changed files will appear here.</p>
    </div>
  {:else}
    <section class="commit-summary">
      <div class="commit-summary-heading">
        <button class="copy-layer copy-heading" type="button" title="Copy commit message" on:click={() => void copyLayer(selected.message, 'Commit message')}>
          <h2 class="copy-target">{title(selected.message)}</h2>
        </button>
        {#if isMergeCommit}<span class="merge-commit-badge">Merge</span>{/if}
      </div>
      <button class="copy-layer sha-line" type="button" title="Copy commit hash" on:click={() => void copyLayer(selected.commit, 'Commit hash')}><code class="copy-target">{selected.commit}</code></button>
      <dl>
        <dt>Author</dt>
        <dd><button class="copy-layer metadata-copy" type="button" title="Copy author" on:click={() => void copyLayer(`${selected.author.name} <${selected.author.email}>`, 'Author')}><span class="copy-target">{selected.author.name} <small>&lt;{selected.author.email}&gt;</small></span></button></dd>
        <dt>Date</dt>
        <dd><button class="copy-layer metadata-copy" type="button" title="Copy date" on:click={() => void copyLayer(formatDate(selected.date, true), 'Date')}><span class="copy-target">{formatDate(selected.date, true)}</span></button></dd>
        <dt>Refs</dt>
        <dd class="ref-list">
          {#if selectedRefs.length}
            {#each selectedRefs as ref}<button class="copy-layer" type="button" title="Copy ref" on:click={() => void copyLayer(ref, 'Ref')}><span class="copy-target">{ref}</span></button>{/each}
          {:else}
            <span class="muted">none</span>
          {/if}
        </dd>
        {#if isMergeCommit}
          <dt>Parents</dt>
          <dd class="ref-list parent-list">
            {#each selectedParents as parent}<button class="copy-layer" type="button" title="Copy parent commit" on:click={() => void copyLayer(parent, 'Parent commit')}><code class="copy-target">{parent.slice(0, 8)}</code></button>{/each}
          </dd>
        {/if}
        {#if reviewLink}
          <dt>Review</dt>
          <dd><button class="review-link" type="button" title={reviewLink.url} on:click={() => onOpenExternalURL(reviewLink?.url ?? '')}>↗ {reviewLink.label}</button></dd>
        {/if}
      </dl>
      <div class="inspector-actions">
        <button type="button" on:click={() => onOpenIDE(selected.file.path)} disabled={!selected.file.path}>↗ Open IDE</button>
        <button type="button" on:click={() => onOpenFinder(selected.file.path)} disabled={!selected.file.path}>▱ Open Finder</button>
      </div>
    </section>

    <section class="changed-files">
      <div class="inspector-section-title">
        <span class="changed-files-heading">Changed files <span>({selected.files.length})</span></span>
        <div class="changed-files-view-toggle" role="group" aria-label="Changed files layout">
          <button class:active={changedFilesView === 'list'} type="button" aria-pressed={changedFilesView === 'list'} on:click={() => setChangedFilesView('list')}>List</button>
          <button class:active={changedFilesView === 'tree'} type="button" aria-pressed={changedFilesView === 'tree'} on:click={() => setChangedFilesView('tree')}>Tree</button>
        </div>
      </div>
      {#if selected.files.length > 0 && changedFilesView === 'list'}
        <div class="changed-file-list" on:scroll={() => closeDiffPopover()}>
          {#each selected.files as file}
            <button
              type="button"
              on:click={(event) => void openDiffPopover(event, file)}
              on:contextmenu|preventDefault={(event) => openPathContextMenu(event, file.path)}
              class:selected={diffFile?.path === file.path}
              class:interaction-active={contextMenu?.targetPath === file.path}
              class="changed-file-row"
              aria-expanded={diffFile?.path === file.path}
            >
              <span class="file-status status-{file.status[0]?.toLowerCase()}">{file.status[0]}</span>
              <span class="copy-target" title={fileLabel(file)}>{fileLabel(file)}</span>
            </button>
          {/each}
        </div>
      {:else if selected.files.length > 0}
        <div class="changed-file-list changed-file-tree" role="tree" aria-label="Changed files tree" on:scroll={() => closeDiffPopover()}>
          {#each visibleChangedNodes as visible (visible.node.path)}
            {@const node = visible.node}
            {#if node.kind === 'directory'}
              <button
                class:interaction-active={contextMenu?.targetPath === node.path}
                class="changed-tree-row directory"
                type="button"
                role="treeitem"
                aria-level={visible.depth + 1}
                aria-selected="false"
                aria-expanded={!collapsedDirectories.has(node.path)}
                style={`padding-left: ${8 + visible.depth * 15}px`}
                on:click={() => toggleChangedDirectory(node.path)}
                on:contextmenu|preventDefault={(event) => openPathContextMenu(event, node.path, `${node.path}/**`)}
              >
                <svg class:expanded={!collapsedDirectories.has(node.path)} class="tree-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m6 4 4 4-4 4" /></svg>
                <svg class="tree-entry-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M2.5 4.5h4l1.5 2h5.5v6.5h-11z" /></svg>
                <span class="copy-target" title={node.path}>{node.name}</span>
              </button>
            {:else if node.file}
              <button
                class:selected={diffFile?.path === node.file.path}
                class:interaction-active={contextMenu?.targetPath === node.file.path}
                class="changed-tree-row file"
                type="button"
                role="treeitem"
                aria-level={visible.depth + 1}
                aria-selected={diffFile?.path === node.file.path}
                aria-expanded={diffFile?.path === node.file.path}
                style={`padding-left: ${8 + visible.depth * 15}px`}
                on:click={(event) => void openDiffPopover(event, node.file as FileChange)}
                on:contextmenu|preventDefault={(event) => openPathContextMenu(event, node.file?.path ?? '')}
              >
                <span class="tree-chevron-spacer"></span>
                <span class="file-status status-{node.file.status[0]?.toLowerCase()}">{node.file.status[0]}</span>
                <span class="copy-target" title={node.file.path}>{node.name}</span>
              </button>
            {/if}
          {/each}
        </div>
      {:else}
        <div class="changed-files-empty">No changed files in this commit.</div>
      {/if}
    </section>
  {/if}
  {#if copyToast}<div class:failed={copyToast.startsWith('Failed')} class="copy-toast" role="status" aria-live="polite">{copyToast}</div>{/if}
</aside>

{#if diffFile && popoverPosition}
  <div
    class="diff-popover"
    role="dialog"
    aria-modal="false"
    aria-labelledby="diff-popover-title"
    tabindex="-1"
    style={`top: ${popoverPosition.top}px; left: ${popoverPosition.left}px; width: ${popoverPosition.width}px; max-height: ${popoverPosition.maxHeight}px; --diff-arrow-top: ${popoverPosition.arrowTop}px`}
    on:mousedown|stopPropagation
  >
    <header class="diff-popover-header">
      <div>
        <span class="file-status status-{diffFile.status[0]?.toLowerCase()}">{diffFile.status[0]}</span>
        <button id="diff-popover-title" class="copy-layer" type="button" title="Copy file path" on:click={() => void copyLayer(fileLabel(diffFile as FileChange), 'File path')}><code class="copy-target">{fileLabel(diffFile)}</code></button>
      </div>
      <button class="diff-popover-close" type="button" aria-label="Close file changes" on:click={closeDiffPopover}>×</button>
    </header>
    <div class="diff-popover-content">
      {#if diffLoading}
        <div class="diff-popover-state"><span class="empty-spinner"></span><strong>Loading changes…</strong></div>
      {:else if diffLines.length === 0}
        <div class="diff-popover-state"><strong>No added or removed lines</strong><span>This file may only have been renamed, changed as binary data, or left unchanged by a merge.</span></div>
      {:else}
        <div class="diff-popover-lines">
          {#each diffLines as line}
            <div class="diff-popover-line {line.kind}"><code>{line.text}</code></div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

{#if contextMenu}
  <ContextMenu x={contextMenu.x} y={contextMenu.y} items={contextMenu.items} ariaLabel={contextMenu.label} onClose={() => (contextMenu = null)} />
{/if}
