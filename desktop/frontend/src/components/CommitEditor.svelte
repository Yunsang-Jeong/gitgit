<script lang="ts">
  import { tick } from 'svelte'
  import { buildChangedFileTree, type ChangedFileTreeNode } from '../lib/changed-files'
  import type { ChangedFilesView, CommitEditCommit, CommitEditStack, CommitFileContent, FileChange, RewriteCommitsRequest } from '../lib/types'

  type FileDraft = CommitFileContent & {
    originalContent: string
    originalDelete: boolean
    dirty: boolean
  }
  type VisibleChangedNode = { node: ChangedFileTreeNode; depth: number }

  export let open = false
  export let stack: CommitEditStack | null = null
  export let loading = false
  export let applying = false
  export let error = ''
  export let onClose: () => void
  export let onLoadFile: (commit: string, path: string) => Promise<CommitFileContent>
  export let onApply: (request: RewriteCommitsRequest) => Promise<void>

  let observedHead = ''
  let draftCommits: CommitEditCommit[] = []
  let selectedCommit = ''
  let selectedFilePath = ''
  let fileDrafts: Record<string, FileDraft> = {}
  let fileLoadingKey = ''
  let defaultBranchConfirmed = false
  let changesView: ChangedFilesView = 'tree'
  let collapsedDirectories = new Set<string>()
  let discardConfirmationOpen = false
  let keepEditingButton: HTMLButtonElement

  $: stackHead = stack?.head ?? ''
  $: if (stackHead !== observedHead) initialiseDraft()
  $: selected = draftCommits.find((commit) => commit.commit === selectedCommit) ?? null
  $: selectedFile = selected?.files.find((file) => file.path === selectedFilePath) ?? null
  $: selectedFileDraft = selected ? fileDrafts[fileKey(selected.commit, selectedFilePath)] ?? null : null
  $: changedFileTree = buildChangedFileTree(selected?.files ?? [])
  $: visibleChangedNodes = flattenChangedFileTree(changedFileTree)
  $: fileEditCount = Object.values(fileDrafts).filter((draft) => draft.dirty).length
  $: messageEditCount = stack ? draftCommits.filter((commit) => stack?.commits.find((original) => original.commit === commit.commit)?.message !== commit.message).length : 0
  $: orderChanged = Boolean(stack && draftCommits.some((commit, index) => stack?.commits[index]?.commit !== commit.commit))
  $: hasChanges = orderChanged || messageEditCount > 0 || fileEditCount > 0
  $: canApply = Boolean(stack && hasChanges && !loading && !applying && (!stack.default_branch_target || defaultBranchConfirmed))

  function initialiseDraft(): void {
    observedHead = stackHead
    draftCommits = stack?.commits.map((commit) => ({ ...commit, files: commit.files.map((file) => ({ ...file })) })) ?? []
    selectedCommit = draftCommits[0]?.commit ?? ''
    selectedFilePath = ''
    fileDrafts = {}
    fileLoadingKey = ''
    defaultBranchConfirmed = false
    changesView = 'tree'
    collapsedDirectories = new Set<string>()
    discardConfirmationOpen = false
  }

  function title(message: string): string {
    return message.split('\n')[0]
  }

  function fileLabel(file: FileChange): string {
    return file.old_path ? `${file.old_path} → ${file.path}` : file.path
  }

  function fileKey(commit: string, path: string): string {
    return `${commit}\u0000${path}`
  }

  function chooseCommit(commit: CommitEditCommit): void {
    selectedCommit = commit.commit
    selectedFilePath = ''
    collapsedDirectories = new Set<string>()
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
  }

  function moveCommit(index: number, offset: number): void {
    const target = index + offset
    if (target < 0 || target >= draftCommits.length) return
    const next = [...draftCommits]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    draftCommits = next
  }

  function updateMessage(value: string): void {
    draftCommits = draftCommits.map((commit) => commit.commit === selectedCommit ? { ...commit, message: value } : commit)
  }

  async function chooseFile(file: FileChange): Promise<void> {
    if (!selected) return
    const commitOID = selected.commit
    selectedFilePath = file.path
    const key = fileKey(commitOID, file.path)
    if (fileDrafts[key] || fileLoadingKey === key) return
    fileLoadingKey = key
    try {
      const content = await onLoadFile(commitOID, file.path)
      fileDrafts = {
        ...fileDrafts,
        [key]: {
          ...content,
          originalContent: content.content,
          originalDelete: !content.exists,
          dirty: false,
        },
      }
    } catch (error) {
      fileDrafts = {
        ...fileDrafts,
        [key]: {
          commit: commitOID,
          path: file.path,
          content: '',
          exists: false,
          editable: false,
          reason: error instanceof Error ? error.message : typeof error === 'string' ? error : 'Failed to read file content.',
          originalContent: '',
          originalDelete: true,
          dirty: false,
        },
      }
    } finally {
      if (fileLoadingKey === key) fileLoadingKey = ''
    }
  }

  function updateFileContent(content: string): void {
    if (!selectedFileDraft || !selected) return
    const key = fileKey(selected.commit, selectedFileDraft.path)
    const next = { ...selectedFileDraft, content }
    next.dirty = next.content !== next.originalContent || !next.exists !== next.originalDelete
    fileDrafts = { ...fileDrafts, [key]: next }
  }

  function updateFileDelete(deleteFile: boolean): void {
    if (!selectedFileDraft || !selected) return
    const key = fileKey(selected.commit, selectedFileDraft.path)
    const next = { ...selectedFileDraft, exists: !deleteFile }
    next.dirty = next.content !== next.originalContent || deleteFile !== next.originalDelete
    fileDrafts = { ...fileDrafts, [key]: next }
  }

  function restoreFileDraft(): void {
    if (!selectedFileDraft || !selected) return
    const key = fileKey(selected.commit, selectedFileDraft.path)
    fileDrafts = {
      ...fileDrafts,
      [key]: {
        ...selectedFileDraft,
        content: selectedFileDraft.originalContent,
        exists: !selectedFileDraft.originalDelete,
        dirty: false,
      },
    }
  }

  function applyRewrite(): void {
    if (!stack || !canApply) return
    const request: RewriteCommitsRequest = {
      branch: stack.branch,
      expected_head: stack.head,
      base: stack.base,
      confirm_default_branch: stack.default_branch_target && defaultBranchConfirmed,
      commits: draftCommits.map((commit) => ({
        commit: commit.commit,
        message: commit.message,
        file_edits: Object.values(fileDrafts)
          .filter((draft) => draft.commit === commit.commit && draft.dirty)
          .map((draft) => ({ path: draft.path, content: draft.content, delete: !draft.exists })),
      })),
    }
    void onApply(request)
  }

  async function requestClose(): Promise<void> {
    if (applying) return
    if (!hasChanges) {
      onClose()
      return
    }
    discardConfirmationOpen = true
    await tick()
    keepEditingButton?.focus()
  }

  function discardEdits(): void {
    discardConfirmationOpen = false
    onClose()
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (!open || event.key !== 'Escape' || applying) return
    event.preventDefault()
    if (discardConfirmationOpen) {
      discardConfirmationOpen = false
      return
    }
    void requestClose()
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <div class="commit-editor-backdrop" role="presentation" on:mousedown={() => void requestClose()}>
    <div class="commit-editor-modal" role="dialog" aria-modal="true" aria-labelledby="commit-editor-title" tabindex="-1" on:mousedown|stopPropagation>
      <header class="commit-editor-header">
        <div>
          <h1 id="commit-editor-title">Edit commits</h1>
          {#if stack}<span><code>{stack.branch}</code> · selected commit through HEAD</span>{:else}<span>Preparing editable history…</span>{/if}
        </div>
        <button type="button" on:click={() => void requestClose()} disabled={applying} aria-label="Close commit editor">×</button>
      </header>

      {#if loading}
        <div class="commit-editor-state"><span class="empty-spinner"></span><strong>Reading commit stack…</strong></div>
      {:else if !stack}
        <div class="commit-editor-state failed"><strong>Commit editor unavailable</strong><span>{error || 'Select a commit and try again.'}</span></div>
      {:else}
        {#if stack.default_branch_target}
          <div class="commit-editor-warning default-branch-warning" role="alert">
            <strong>Warning: <code>{stack.branch}</code> is the default branch.</strong>
            <span>This operation rewrites default-branch history and changes commit hashes. A force push may affect every collaborator.</span>
            <label><input type="checkbox" bind:checked={defaultBranchConfirmed} disabled={applying} /> I understand and want to rewrite the default branch.</label>
          </div>
        {:else}
          <div class="commit-editor-warning">
            <strong>Commit hashes will change.</strong>
            <span>GitGit rebuilds this stack in a temporary worktree and keeps the original HEAD under a backup ref.</span>
          </div>
        {/if}

        <div class="commit-editor-content">
          <aside class="commit-editor-stack">
            <header>
              <strong>Order</strong>
              <span>Oldest first · {draftCommits.length} commit{draftCommits.length === 1 ? '' : 's'}</span>
            </header>
            <div class="commit-editor-stack-list">
              {#each draftCommits as commit, index (commit.commit)}
                <div class:selected={selectedCommit === commit.commit} class="commit-editor-stack-row">
                  <button class="commit-editor-stack-select" type="button" on:click={() => chooseCommit(commit)}>
                    <span>{index + 1}</span>
                    <span><strong>{title(commit.message) || 'Empty message'}</strong><code>{commit.short_commit}</code></span>
                  </button>
                  <div class="commit-editor-order-actions">
                    <button type="button" on:click={() => moveCommit(index, -1)} disabled={index === 0 || applying} aria-label={`Move ${commit.short_commit} earlier`}>↑</button>
                    <button type="button" on:click={() => moveCommit(index, 1)} disabled={index === draftCommits.length - 1 || applying} aria-label={`Move ${commit.short_commit} later`}>↓</button>
                  </div>
                </div>
              {/each}
            </div>
          </aside>

          <section class="commit-editor-detail">
            {#if selected}
              <label class="commit-editor-message">
                <span>Message</span>
                <textarea value={selected.message} on:input={(event) => updateMessage((event.currentTarget as HTMLTextAreaElement).value)} disabled={applying} spellcheck="false"></textarea>
              </label>

              <div class="commit-editor-files">
                <header>
                  <div class="commit-editor-files-heading"><strong>Changes</strong><span>{selected.files.length} files · select one to edit its resulting content</span></div>
                  <div class="commit-editor-files-layout" role="group" aria-label="Changes layout">
                    <button class:active={changesView === 'list'} type="button" aria-pressed={changesView === 'list'} on:click={() => (changesView = 'list')}>List</button>
                    <button class:active={changesView === 'tree'} type="button" aria-pressed={changesView === 'tree'} on:click={() => (changesView = 'tree')}>Tree</button>
                  </div>
                </header>
                {#if selected.files.length > 0 && changesView === 'list'}
                  <div class="commit-editor-file-list" aria-label="Changed files list">
                    {#each selected.files as file (file.path)}
                      <button class:selected={selectedFilePath === file.path} class="commit-editor-file-row" type="button" on:click={() => void chooseFile(file)} disabled={applying}>
                        <span class="file-status status-{file.status[0]?.toLowerCase()}">{file.status[0]}</span>
                        <code title={fileLabel(file)}>{fileLabel(file)}</code>
                        {#if fileDrafts[fileKey(selected.commit, file.path)]?.dirty}<small>edited</small>{/if}
                      </button>
                    {/each}
                  </div>
                {:else if selected.files.length > 0}
                  <div class="commit-editor-file-list commit-editor-file-tree" role="tree" aria-label="Changed files tree">
                    {#each visibleChangedNodes as visible (visible.node.path)}
                      {@const node = visible.node}
                      {#if node.kind === 'directory'}
                        <button
                          class="commit-editor-tree-row directory"
                          type="button"
                          role="treeitem"
                          aria-level={visible.depth + 1}
                          aria-selected="false"
                          aria-expanded={!collapsedDirectories.has(node.path)}
                          style={`padding-left: ${8 + visible.depth * 15}px`}
                          on:click={() => toggleChangedDirectory(node.path)}
                        >
                          <svg class:expanded={!collapsedDirectories.has(node.path)} class="tree-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m6 4 4 4-4 4" /></svg>
                          <svg class="tree-entry-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M2.5 4.5h4l1.5 2h5.5v6.5h-11z" /></svg>
                          <code title={node.path}>{node.name}</code>
                        </button>
                      {:else if node.file}
                        <button
                          class:selected={selectedFilePath === node.file.path}
                          class="commit-editor-tree-row file"
                          type="button"
                          role="treeitem"
                          aria-level={visible.depth + 1}
                          aria-selected={selectedFilePath === node.file.path}
                          style={`padding-left: ${8 + visible.depth * 15}px`}
                          on:click={() => void chooseFile(node.file as FileChange)}
                          disabled={applying}
                        >
                          <span class="tree-chevron-spacer"></span>
                          <span class="file-status status-{node.file.status[0]?.toLowerCase()}">{node.file.status[0]}</span>
                          <code title={fileLabel(node.file)}>{node.name}</code>
                          {#if fileDrafts[fileKey(selected.commit, node.file.path)]?.dirty}<small>edited</small>{/if}
                        </button>
                      {/if}
                    {/each}
                  </div>
                {:else}
                  <span class="commit-editor-no-files">No first-parent file changes in this commit.</span>
                {/if}
              </div>

              {#if selectedFile}
                <div class="commit-editor-file-content">
                  <header>
                    <code>{fileLabel(selectedFile)}</code>
                    {#if selectedFileDraft?.dirty}<button type="button" on:click={restoreFileDraft} disabled={applying}>Restore</button>{/if}
                  </header>
                  {#if fileLoadingKey === fileKey(selected.commit, selectedFile.path)}
                    <div class="commit-editor-file-state"><span class="empty-spinner"></span><span>Reading file content…</span></div>
                  {:else if selectedFileDraft && !selectedFileDraft.editable}
                    <div class="commit-editor-file-state failed"><strong>File cannot be edited</strong><span>{selectedFileDraft.reason}</span></div>
                  {:else if selectedFileDraft}
                    <label class="commit-editor-delete"><input type="checkbox" checked={!selectedFileDraft.exists} on:change={(event) => updateFileDelete((event.currentTarget as HTMLInputElement).checked)} disabled={applying} /> Delete file in this commit</label>
                    <textarea value={selectedFileDraft.content} on:input={(event) => updateFileContent((event.currentTarget as HTMLTextAreaElement).value)} disabled={applying || !selectedFileDraft.exists} spellcheck="false"></textarea>
                    {#if selectedFileDraft.reason}<small>{selectedFileDraft.reason}</small>{/if}
                  {/if}
                </div>
              {/if}
            {/if}
          </section>
        </div>

        <footer class="commit-editor-footer">
          <div>
            {#if error}<span class="commit-editor-error">{error}</span>{:else}<span>{orderChanged ? 'Order changed · ' : ''}{messageEditCount} message edit{messageEditCount === 1 ? '' : 's'} · {fileEditCount} file edit{fileEditCount === 1 ? '' : 's'}</span>{/if}
          </div>
          <button type="button" on:click={() => void requestClose()} disabled={applying}>Cancel</button>
          <button class="primary danger" type="button" on:click={applyRewrite} disabled={!canApply}>{applying ? 'Rewriting…' : `Rewrite ${draftCommits.length} commit${draftCommits.length === 1 ? '' : 's'}`}</button>
        </footer>
      {/if}
      {#if discardConfirmationOpen}
        <div class="worktree-confirm-backdrop" role="presentation" on:mousedown|stopPropagation>
          <div class="worktree-confirm" role="alertdialog" aria-modal="true" aria-labelledby="discard-commit-edits-title" tabindex="-1">
            <h2 id="discard-commit-edits-title">Discard commit edits?</h2>
            <p>Your reordered commits, message edits, and file edits have not been applied. Closing the editor will discard this draft.</p>
            <div class="worktree-confirm-actions">
              <button bind:this={keepEditingButton} type="button" on:click={() => (discardConfirmationOpen = false)}>Keep editing</button>
              <button class="danger" type="button" on:click={discardEdits}>Discard edits</button>
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
