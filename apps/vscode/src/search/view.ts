import * as path from 'node:path'

import * as vscode from 'vscode'

import type { SearchRequest, SearchResponse } from '../helper/protocol.js'
import {
  buildSearchRequest,
  initialSearchState,
  reduceSearchState,
  validateSearchDraft,
  type SearchDraft,
  type SearchViewState,
} from './model.js'
import {
  searchFilterSummary,
  searchFileSelection,
  searchStatusMessage,
  searchTree,
  type SearchCommitNode,
  type SearchFileNode,
  type SearchGroupNode,
  type SearchTreeNode,
} from './tree-model.js'

export interface SearchRuntime {
  readonly state: {
    phase: string
    message: string
    repository?: { root: string, branch: string }
  }
  readonly onDidChange: vscode.Event<void>
  runSearch(
    request: SearchRequest,
    onProgress: (progress: { requestId: number, scanned: number, total: number }) => void,
    token: vscode.CancellationToken,
  ): Promise<SearchResponse>
}

type SearchFilter = 'scope' | 'engine' | 'author' | 'since' | 'until' | 'followRename' | 'clear'

interface SearchFilterItem extends vscode.QuickPickItem {
  id: SearchFilter
}

interface SearchKindItem extends vscode.QuickPickItem {
  prefix: string
}

export class SearchViewProvider implements vscode.TreeDataProvider<SearchTreeNode>, vscode.Disposable {
  private view: vscode.TreeView<SearchTreeNode> | undefined
  private searchState: SearchViewState = initialSearchState()
  private cancellation: vscode.CancellationTokenSource | undefined
  private nextRequestId = 1
  private repositoryRoot: string | undefined
  private nodes: SearchCommitNode[] = []
  private readonly subscriptions: vscode.Disposable[] = []
  private readonly changed = new vscode.EventEmitter<SearchTreeNode | undefined>()
  readonly onDidChangeTreeData = this.changed.event

  constructor(private readonly runtime: SearchRuntime) {
    this.repositoryRoot = runtime.state.repository?.root
    this.subscriptions.push(runtime.onDidChange(() => this.runtimeChanged()))
  }

  attachView(view: vscode.TreeView<SearchTreeNode>): void {
    this.view = view
    this.updateView()
  }

  getTreeItem(element: SearchTreeNode): vscode.TreeItem {
    switch (element.kind) {
      case 'commit':
        return commitTreeItem(element)
      case 'group':
        return groupTreeItem(element)
      case 'file':
        return fileTreeItem(element)
    }
  }

  getChildren(element?: SearchTreeNode): SearchTreeNode[] {
    if (!element) return this.nodes
    return element.kind === 'file' ? [] : element.children
  }

  async promptAndSearch(): Promise<void> {
    let value = this.searchState.draft.expression
    if (!value.trim()) {
      const kind = await vscode.window.showQuickPick<SearchKindItem>([
        { label: '$(comment-discussion) Message', description: 'Search commit messages', prefix: 'MSG: ' },
        { label: '$(diff) Diff', description: 'Search added and removed lines', prefix: 'DIFF: ' },
        { label: '$(file) File path', description: 'Search changed paths', prefix: 'FILE: ' },
        { label: '$(symbol-operator) Advanced expression', description: 'Combine MSG:, DIFF:, and FILE:', prefix: '' },
      ], {
        title: 'GitGit Search',
        placeHolder: 'Choose what to search',
      })
      if (!kind) return
      value = kind.prefix
    }

    const draft = this.searchState.draft
    const expression = await vscode.window.showInputBox({
      title: 'GitGit Search',
      prompt: `Search commit history · ${searchFilterSummary(draft)}`,
      placeHolder: 'MSG: *cache* OR FILE: **/*.go',
      value,
      valueSelection: [value.length, value.length],
      validateInput: (candidate) => validateSearchDraft({ ...draft, expression: candidate }).error || undefined,
    })
    if (expression === undefined) return
    await this.search({ ...draft, expression })
  }

  async configureFilters(): Promise<void> {
    const draft = this.searchState.draft
    const selected = await vscode.window.showQuickPick<SearchFilterItem>([
      { id: 'scope', label: '$(git-branch) Scope', description: draft.allRefs ? 'All refs' : 'HEAD' },
      { id: 'engine', label: '$(regex) Matching', description: draft.engine === 'regex' ? 'Regex' : 'Glob' },
      { id: 'author', label: '$(account) Author', description: draft.author.trim() || 'Any author' },
      { id: 'since', label: '$(calendar) Since', description: draft.since.trim() || 'No lower date bound' },
      { id: 'until', label: '$(calendar) Until', description: draft.until.trim() || 'No upper date bound' },
      {
        id: 'followRename',
        label: '$(references) Follow renames',
        description: draft.followRename ? 'Enabled' : 'Disabled',
      },
      { id: 'clear', label: '$(clear-all) Clear filters', description: 'Restore HEAD and Glob defaults' },
    ], {
      title: 'GitGit Search Filters',
      placeHolder: searchFilterSummary(draft),
      matchOnDescription: true,
    })
    if (!selected) return

    let next: SearchDraft | undefined
    switch (selected.id) {
      case 'scope': {
        const scope = await vscode.window.showQuickPick([
          { label: 'HEAD', description: 'Search the current history scope', allRefs: false },
          { label: 'All refs', description: 'Include branches and tags', allRefs: true },
        ], { title: 'GitGit Search Scope' })
        if (scope) next = { ...draft, allRefs: scope.allRefs }
        break
      }
      case 'engine': {
        const engine = await vscode.window.showQuickPick([
          { label: 'Glob', description: 'Wildcard matching', engine: 'glob' as const },
          { label: 'Regex', description: 'Regular expression matching', engine: 'regex' as const },
        ], { title: 'GitGit Search Matching' })
        if (engine) next = { ...draft, engine: engine.engine }
        break
      }
      case 'author': {
        const author = await filterInput('Author', 'Name or email', draft.author)
        if (author !== undefined) next = { ...draft, author }
        break
      }
      case 'since': {
        const since = await filterInput('Since', 'last:30d or 2026. 08. 15.', draft.since)
        if (since !== undefined) next = { ...draft, since }
        break
      }
      case 'until': {
        const until = await filterInput('Until', '2026. 08. 15.', draft.until)
        if (until !== undefined) next = { ...draft, until }
        break
      }
      case 'followRename': {
        const follow = await vscode.window.showQuickPick([
          { label: 'Disabled', description: 'Match the path at each commit', enabled: false },
          { label: 'Enabled', description: 'Trace matching FILE: paths across renames', enabled: true },
        ], { title: 'Follow Renames' })
        if (follow) next = { ...draft, followRename: follow.enabled }
        break
      }
      case 'clear':
        next = {
          ...draft,
          engine: 'glob',
          allRefs: false,
          author: '',
          since: '',
          until: '',
          followRename: false,
        }
        break
    }
    if (next) this.edit(next)
  }

  async rerun(): Promise<void> {
    if (!this.searchState.draft.expression.trim()) {
      await this.promptAndSearch()
      return
    }
    await this.search(this.searchState.draft)
  }

  cancel(): void {
    const requestId = this.searchState.activeRequestId
    if (requestId === undefined) return
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = undefined
    this.searchState = reduceSearchState(this.searchState, { type: 'cancel', requestId })
    this.updateView()
  }

  clear(): void {
    const draft = this.searchState.draft
    this.cancelActiveSearch()
    this.searchState = { ...initialSearchState(), draft }
    this.nodes = []
    this.changed.fire(undefined)
    this.updateView()
  }

  dispose(): void {
    this.cancelActiveSearch()
    this.changed.dispose()
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private edit(draft: SearchDraft): void {
    this.searchState = reduceSearchState(this.searchState, { type: 'edit', draft })
    this.updateView()
  }

  private async search(draft: SearchDraft): Promise<void> {
    // A new attempt supersedes the previous request even when the replacement
    // draft or runtime is invalid. Clear the active request first so a late
    // response cannot settle into the replacement attempt.
    this.cancelActiveSearch()
    const validated = validateSearchDraft(draft)
    this.edit(draft)
    if (validated.error) {
      this.searchState = { ...this.searchState, error: validated.error, phase: 'error' }
      this.updateView()
      return
    }
    const repository = this.runtime.state.repository
    if (!repository || this.runtime.state.phase !== 'ready') {
      this.searchState = { ...this.searchState, error: this.runtime.state.message, phase: 'error' }
      this.updateView()
      return
    }

    const cancellation = new vscode.CancellationTokenSource()
    this.cancellation = cancellation
    const requestId = this.nextRequestId++
    const request = buildSearchRequest(repository.root, requestId, draft, validated.patterns)
    this.searchState = reduceSearchState(this.searchState, { type: 'start', requestId, draft })
    this.updateView()

    try {
      const response = await this.runtime.runSearch(
        request,
        (progress) => {
          this.searchState = reduceSearchState(this.searchState, { type: 'progress', progress })
          this.updateView()
        },
        cancellation.token,
      )
      this.searchState = reduceSearchState(this.searchState, { type: 'success', requestId, draft, response })
      this.nodes = searchTree(this.searchState.results)
      this.changed.fire(undefined)
    } catch (error) {
      if (!cancellation.token.isCancellationRequested) {
        const message = error instanceof Error ? error.message : String(error)
        this.searchState = reduceSearchState(this.searchState, { type: 'failure', requestId, message })
      }
    } finally {
      if (this.cancellation === cancellation) {
        cancellation.dispose()
        this.cancellation = undefined
      }
      this.updateView()
    }
  }

  private runtimeChanged(): void {
    const nextRoot = this.runtime.state.phase === 'ready' ? this.runtime.state.repository?.root : undefined
    if (nextRoot && this.repositoryRoot && nextRoot !== this.repositoryRoot) {
      const draft = this.searchState.draft
      this.cancelActiveSearch()
      this.searchState = { ...initialSearchState(), draft }
      this.nodes = []
      this.changed.fire(undefined)
    } else if (this.runtime.state.phase !== 'ready' && this.runtime.state.phase !== 'loading') {
      const draft = this.searchState.draft
      this.cancelActiveSearch()
      this.searchState = { ...initialSearchState(), draft }
      this.repositoryRoot = undefined
      this.nodes = []
      this.changed.fire(undefined)
    }
    if (nextRoot) this.repositoryRoot = nextRoot
    this.updateView()
  }

  private cancelActiveSearch(): void {
    const requestId = this.searchState.activeRequestId
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = undefined
    if (requestId !== undefined) {
      this.searchState = reduceSearchState(this.searchState, { type: 'cancel', requestId })
    }
  }

  private updateView(): void {
    const view = this.view
    const repository = this.runtime.state.repository
    if (view) {
      view.description = repository
        ? `${path.basename(repository.root)} · ${repository.branch || 'detached'}`
        : undefined
      view.badge = this.searchState.results.length > 0
        ? { value: this.searchState.results.length, tooltip: `${this.searchState.results.length} matching commits` }
        : undefined
      view.message = searchStatusMessage(this.searchState, this.runtime.state)
    }
    void vscode.commands.executeCommand('setContext', 'gitgit.searchRunning', this.searchState.phase === 'running')
    void vscode.commands.executeCommand('setContext', 'gitgit.searchHasResults', this.searchState.results.length > 0)
  }

}

function commitTreeItem(node: SearchCommitNode): vscode.TreeItem {
  const item = new vscode.TreeItem(
    node.label,
    node.children.length > 0 ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
  )
  item.id = node.id
  item.description = node.description
  item.iconPath = new vscode.ThemeIcon('git-commit')
  item.contextValue = 'gitgit.searchCommit'
  item.accessibilityInformation = { label: node.accessibilityLabel, role: 'treeitem' }
  const tooltip = new vscode.MarkdownString(undefined, true)
  tooltip.appendMarkdown('$(git-commit) **GitGit Search**\n\n')
  tooltip.appendText(node.result.message)
  tooltip.appendMarkdown('\n\n')
  tooltip.appendText(`Author: ${node.result.author.name}\nDate: ${node.result.date}\nCommit: ${node.result.commit}`)
  if (node.result.refs.length > 0) {
    tooltip.appendMarkdown('\n\n')
    tooltip.appendText(`Refs: ${node.result.refs.join(', ')}`)
  }
  item.tooltip = tooltip
  return item
}

function groupTreeItem(node: SearchGroupNode): vscode.TreeItem {
  const item = new vscode.TreeItem(node.label, vscode.TreeItemCollapsibleState.Expanded)
  item.id = node.id
  item.description = node.description
  item.iconPath = new vscode.ThemeIcon(node.group === 'matched' ? 'search' : 'files')
  item.contextValue = 'gitgit.searchGroup'
  return item
}

function fileTreeItem(node: SearchFileNode): vscode.TreeItem {
  const item = new vscode.TreeItem(node.label, vscode.TreeItemCollapsibleState.None)
  item.id = node.id
  item.description = node.description
  item.iconPath = fileStatusIcon(node.file.status)
  item.contextValue = node.group === 'matched' ? 'gitgit.searchMatchedFile' : 'gitgit.searchChangedFile'
  item.accessibilityInformation = { label: node.accessibilityLabel, role: 'treeitem' }
  const action = searchFileSelection(node)
  if (action) {
    item.command = {
      command: 'gitgit.commit.select',
      title: 'Open Revision',
      arguments: [action],
    }
  }
  const tooltip = new vscode.MarkdownString(undefined, true)
  tooltip.appendMarkdown(node.group === 'matched' ? '**Matched file**\n\n' : '**Changed file**\n\n')
  tooltip.appendText(node.file.oldPath ? `${node.file.oldPath} → ${node.file.path}` : node.file.path)
  tooltip.appendMarkdown('\n\n')
  tooltip.appendText(`Status: ${node.file.status}`)
  if ('matchSources' in node.file) {
    tooltip.appendMarkdown('\n\n')
    tooltip.appendText(`Matched: ${node.file.matchSources.join(', ')}`)
  }
  item.tooltip = tooltip
  return item
}

function fileStatusIcon(status: string): vscode.ThemeIcon {
  switch (status[0]) {
    case 'A': return new vscode.ThemeIcon('diff-added')
    case 'D': return new vscode.ThemeIcon('diff-removed')
    case 'R':
    case 'C': return new vscode.ThemeIcon('diff-renamed')
    default: return new vscode.ThemeIcon('diff-modified')
  }
}

async function filterInput(title: string, placeHolder: string, value: string): Promise<string | undefined> {
  return vscode.window.showInputBox({ title: `GitGit Search: ${title}`, placeHolder, value })
}
