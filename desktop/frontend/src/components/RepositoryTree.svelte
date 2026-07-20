<script lang="ts">
  import { tick } from 'svelte'
  import ContextMenu from './ContextMenu.svelte'
  import { changedDirectoryPaths, changedFilesSignature, changedFileStatusMap } from '../lib/changed-files'
  import type { CommitFilterAction, ContextMenuItem, FileChange, RepositoryTreeEntry, RepositoryTreeResponse } from '../lib/types'

  export let revision = ''
  export let allUsesDefault = false
  export let changedFiles: FileChange[] = []
  export let onLoadTree: (revision: string, directory: string) => Promise<RepositoryTreeResponse>
  export let onCopyPath: (path: string) => void
  export let onAddFilter: (action: CommitFilterAction, path: string) => void
  export let onAddSearch: (path: string) => void
  export let onOpenFinder: (path: string) => void
  export let onOpenTerminal: (path: string) => void

  type TreeNode = RepositoryTreeEntry & {
    expanded: boolean
    loading: boolean
    children?: TreeNode[]
    error?: string
  }

  type VisibleNode = {
    node: TreeNode
    depth: number
  }

  let roots: TreeNode[] = []
  let loading = false
  let error = ''
  let loadedTreeKey = ''
  let treeVersion = 0
  let treeScroll: HTMLElement
  let contextMenu: { x: number; y: number; label: string; items: ContextMenuItem[]; targetPath: string } | null = null

  $: visibleNodes = flatten(roots)
  $: changedStatuses = changedFileStatusMap(changedFiles)
  $: autoExpandDirectories = changedDirectoryPaths(changedFiles)
  $: treeKey = `${revision}\u0000${changedFilesSignature(changedFiles)}`
  $: if (treeKey !== loadedTreeKey) {
    loadedTreeKey = treeKey
    void loadRoot(revision)
  }

  function toNodes(entries: RepositoryTreeEntry[]): TreeNode[] {
    return entries.map((entry) => ({ ...entry, expanded: false, loading: false }))
  }

  function flatten(nodes: TreeNode[], depth = 0): VisibleNode[] {
    const visible: VisibleNode[] = []
    for (const node of nodes) {
      visible.push({ node, depth })
      if (node.expanded && node.children) visible.push(...flatten(node.children, depth + 1))
    }
    return visible
  }

  async function loadRoot(nextRevision: string): Promise<void> {
    const version = ++treeVersion
    roots = []
    error = ''
    if (!nextRevision) {
      loading = false
      return
    }
    loading = true
    try {
      const response = await onLoadTree(nextRevision, '')
      if (version !== treeVersion) return
      roots = toNodes(response.entries)
      await expandChangedDirectories(roots, version)
      if (version !== treeVersion) return
      roots = [...roots]
    } catch (loadError) {
      if (version !== treeVersion) return
      error = errorText(loadError)
    } finally {
      if (version === treeVersion) {
        loading = false
        await revealFirstChangedFile(version)
      }
    }
  }

  async function revealFirstChangedFile(version: number): Promise<void> {
    await tick()
    if (version !== treeVersion || !treeScroll) return
    const changedRow = treeScroll.querySelector<HTMLElement>('.repository-tree-row.file.changed')
    if (!changedRow) return
    const targetTop = changedRow.offsetTop - treeScroll.clientHeight / 2 + changedRow.offsetHeight / 2
    treeScroll.scrollTop = Math.max(0, targetTop)
  }

  async function expandChangedDirectories(nodes: TreeNode[], version: number): Promise<void> {
    const directories = nodes.filter((node) => node.object_type === 'tree' && autoExpandDirectories.has(node.path))
    await Promise.all(directories.map(async (node) => {
      if (version !== treeVersion) return
      node.expanded = true
      node.loading = true
      node.error = ''
      try {
        const response = await onLoadTree(revision, node.path)
        if (version !== treeVersion) return
        node.children = toNodes(response.entries)
        await expandChangedDirectories(node.children, version)
      } catch (loadError) {
        if (version !== treeVersion) return
        node.expanded = false
        node.error = errorText(loadError)
      } finally {
        if (version === treeVersion) node.loading = false
      }
    }))
  }

  async function toggleDirectory(node: TreeNode): Promise<void> {
    if (node.object_type !== 'tree') return
    if (node.children) {
      node.expanded = !node.expanded
      roots = [...roots]
      return
    }

    const version = treeVersion
    node.expanded = true
    node.loading = true
    node.error = ''
    roots = [...roots]
    try {
      const response = await onLoadTree(revision, node.path)
      if (version !== treeVersion) return
      node.children = toNodes(response.entries)
    } catch (loadError) {
      if (version !== treeVersion) return
      node.expanded = false
      node.error = errorText(loadError)
    } finally {
      if (version === treeVersion) {
        node.loading = false
        roots = [...roots]
      }
    }
  }

  function errorText(value: unknown): string {
    if (value instanceof Error) return value.message
    if (typeof value === 'string') return value
    return 'Unable to load this repository tree.'
  }

  function filterPattern(node: TreeNode): string {
    return node.object_type === 'tree' ? `${node.path}/**` : node.path
  }

  function openContextMenu(event: MouseEvent, node: TreeNode): void {
    const pattern = filterPattern(node)
    const filterItems = (['hide', 'show', 'highlight'] as CommitFilterAction[]).map((action, index) => ({
      label: `Add filter: ${action[0].toLocaleUpperCase()}${action.slice(1)}`,
      separatorBefore: index === 0,
      run: () => onAddFilter(action, pattern),
    }))
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      label: `Actions for ${node.path}`,
      targetPath: node.path,
      items: [
        { label: 'Copy path', run: () => onCopyPath(node.path) },
        ...filterItems,
        { label: 'Add search', separatorBefore: true, run: () => onAddSearch(pattern) },
        { label: 'Open Finder', separatorBefore: true, run: () => onOpenFinder(node.path) },
        { label: 'Open Terminal', run: () => onOpenTerminal(node.path) },
      ],
    }
  }
</script>

<section class="repository-tree" aria-label={`Files at ${revision || 'selected branch'}`}>
  <header class="repository-tree-header">
    <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="5" cy="4" r="2" /><circle cx="11" cy="12" r="2" /><path d="M5 6v1.5c0 2.5 6 1.5 6 3" /></svg>
    <div>
      <strong>{revision || 'No branch selected'}</strong>
      <span>{allUsesDefault ? 'Default branch for All branches' : 'Selected branch'}</span>
    </div>
  </header>

  <div bind:this={treeScroll} class="repository-tree-scroll" role="tree" aria-label="Repository files">
    {#if loading}
      <div class="repository-tree-state"><span class="empty-spinner"></span><strong>Loading files…</strong></div>
    {:else if error}
      <div class="repository-tree-state error"><strong>Unable to load files</strong><span>{error}</span></div>
    {:else if !revision}
      <div class="repository-tree-state"><strong>Select a project</strong><span>Repository files will appear here.</span></div>
    {:else if roots.length === 0}
      <div class="repository-tree-state"><strong>Empty repository</strong><span>No tracked files at this branch.</span></div>
    {:else}
      {#each visibleNodes as visible (visible.node.path)}
        {@const node = visible.node}
        {#if node.object_type === 'tree'}
          <button
            class="repository-tree-row directory"
            type="button"
            role="treeitem"
            aria-level={visible.depth + 1}
            aria-expanded={node.expanded}
            aria-selected="false"
            class:interaction-active={contextMenu?.targetPath === node.path}
            title={node.path}
            style={`padding-left: ${10 + visible.depth * 16}px`}
            on:click={() => void toggleDirectory(node)}
            on:contextmenu|preventDefault={(event) => openContextMenu(event, node)}
          >
            <svg class:expanded={node.expanded} class="tree-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m6 4 4 4-4 4" /></svg>
            <svg class="tree-entry-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M2.5 4.5h4l1.5 2h5.5v6.5h-11z" /></svg>
            <span class="copy-target">{node.name}</span>
            {#if node.loading}<span class="tree-row-spinner"></span>{/if}
          </button>
          {#if node.error}<div class="repository-tree-error" style={`padding-left: ${10 + (visible.depth + 1) * 16}px`}>{node.error}</div>{/if}
        {:else}
          <button class:changed={Boolean(changedStatuses[node.path])} class:interaction-active={contextMenu?.targetPath === node.path} class="repository-tree-row file" type="button" role="treeitem" aria-level={visible.depth + 1} aria-selected="false" aria-label={changedStatuses[node.path] ? `${node.path}, ${changedStatuses[node.path]} in selected commit` : node.path} title={changedStatuses[node.path] ? `${node.path} · changed in selected commit` : `Copy ${node.path}`} style={`padding-left: ${10 + visible.depth * 16}px`} on:click={() => onCopyPath(node.path)} on:contextmenu|preventDefault={(event) => openContextMenu(event, node)}>
            <span class="tree-chevron-spacer"></span>
            {#if node.object_type === 'commit'}
              <svg class="tree-entry-icon submodule" viewBox="0 0 16 16" aria-hidden="true"><path d="M3 5.5 8 2.5l5 3v5l-5 3-5-3zM8 8.3v5.2M3.2 5.6 8 8.3l4.8-2.7" /></svg>
            {:else}
              <svg class="tree-entry-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M4 2.5h5l3 3v8H4zM9 2.5v3h3" /></svg>
            {/if}
            <span class="copy-target">{node.name}</span>
            {#if changedStatuses[node.path]}
              <span class="repository-tree-change-status status-{changedStatuses[node.path][0]?.toLowerCase()}" title="Changed in selected commit">{changedStatuses[node.path][0]}</span>
            {/if}
          </button>
        {/if}
      {/each}
    {/if}
  </div>
</section>
{#if contextMenu}
  <ContextMenu x={contextMenu.x} y={contextMenu.y} items={contextMenu.items} ariaLabel={contextMenu.label} onClose={() => (contextMenu = null)} />
{/if}
