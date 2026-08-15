import * as vscode from 'vscode'

import {
  blameAnnotation,
  blameableDocumentLineCount,
  fileHistoryAccessibilityLabel,
  fileHistoryDescription,
  isWorkingTreeCommit,
  normalizedDeletionAnchors,
  normalizedTargetRanges,
  resolveBlameMode,
  revisionContentMatchesDocument,
  traceFileHistorySelections,
  visibleBlameWindow,
  type BlameMode,
} from './model.js'
import type {
  BlameLinesRequest,
  BlameLinesResponse,
  DiffFileRequest,
  DiffFileResponse,
  HistoryCommit,
  HistoryFileRequest,
  HistoryFileResponse,
  RevisionContentRequest,
  RevisionContentResponse,
} from '../helper/protocol.js'
import { repositoryRelativePath } from '../shared/repository.js'
import {
  REVISION_DOCUMENT_SCHEME,
  RevisionRestoreGate,
  revisionSelection,
  shouldUseDeletedPreimage,
  type RevisionDocumentResource,
} from './revision-model.js'
import type { RevisionDocumentProvider } from './revisions.js'
import { openDocumentIfCurrent } from './revision-navigation.js'
import { reviewLinkForCommit } from './review-links.js'

export interface EditorGitClient {
  blameLines(params: BlameLinesRequest, token?: vscode.CancellationToken): Promise<BlameLinesResponse>
  historyFile(params: HistoryFileRequest, token?: vscode.CancellationToken): Promise<HistoryFileResponse>
  revisionContent(params: RevisionContentRequest, token?: vscode.CancellationToken): Promise<RevisionContentResponse>
  diffFile(params: DiffFileRequest, token?: vscode.CancellationToken): Promise<DiffFileResponse>
}

export interface EditorRepositorySession {
  root: string
  head: string
  webRemotes?: string[]
  helper: EditorGitClient
}

export type EditorRepositorySessionProvider = () => EditorRepositorySession | undefined

export interface CommitFileSelection {
  commit: string
  path: string
  oldPath?: string
  status?: string
}

export class LineBlameController implements vscode.Disposable {
  private readonly decoration: vscode.TextEditorDecorationType
  private readonly subscriptions: vscode.Disposable[] = []
  private cancellation: vscode.CancellationTokenSource | undefined
  private timer: NodeJS.Timeout | undefined
  private generation = 0
  private lastDiagnostic = ''

  constructor(
    private readonly session: EditorRepositorySessionProvider,
    private readonly log: (message: string) => void = () => {},
  ) {
    this.decoration = vscode.window.createTextEditorDecorationType({
      after: {
        color: new vscode.ThemeColor('editorCodeLens.foreground'),
        margin: '0 0 0 1.5em',
      },
      rangeBehavior: vscode.DecorationRangeBehavior.ClosedClosed,
    })
    this.subscriptions.push(
      this.decoration,
      vscode.window.onDidChangeActiveTextEditor((active) => {
        for (const editor of vscode.window.visibleTextEditors) {
          if (editor !== active) editor.setDecorations(this.decoration, [])
        }
        this.schedule(0)
      }),
      vscode.window.onDidChangeTextEditorSelection((event) => {
        if (event.textEditor === vscode.window.activeTextEditor) this.schedule(40)
      }),
      vscode.window.onDidChangeTextEditorVisibleRanges((event) => {
        if (event.textEditor === vscode.window.activeTextEditor) this.schedule(80)
      }),
      vscode.workspace.onDidChangeTextDocument((event) => {
        if (event.document === vscode.window.activeTextEditor?.document) this.schedule(220)
      }),
      vscode.workspace.onDidChangeConfiguration((event) => {
        if (event.affectsConfiguration('gitgit.blame')
          || event.affectsConfiguration('git.blame.editorDecoration.enabled')) this.schedule(0)
      }),
      vscode.extensions.onDidChange(() => this.schedule(0)),
    )
  }

  start(): void {
    this.schedule(0)
  }

  clear(): void {
    this.generation++
    this.cancellation?.cancel()
    for (const editor of vscode.window.visibleTextEditors) editor.setDecorations(this.decoration, [])
  }

  refresh(): void {
    this.schedule(0)
  }

  dispose(): void {
    this.generation++
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    if (this.timer) clearTimeout(this.timer)
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private schedule(delay: number): void {
    if (this.timer) clearTimeout(this.timer)
    this.timer = setTimeout(() => {
      this.timer = undefined
      void this.update()
    }, delay)
  }

  private async update(): Promise<void> {
    const generation = ++this.generation
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    if (!editor || !session || editor.document.uri.scheme !== 'file') {
      editor?.setDecorations(this.decoration, [])
      this.diagnose(!editor
        ? '[blame] skipped: no active editor'
        : !session
          ? '[blame] skipped: repository session unavailable'
          : `[blame] skipped: unsupported document scheme ${editor.document.uri.scheme}`)
      return
    }
    const relativePath = repositoryRelativePath(session.root, editor.document.uri.fsPath)
    const configuredMode = vscode.workspace.getConfiguration('gitgit.blame', editor.document.uri)
      .get<BlameMode>('mode', 'auto')
    const nativeBlameEnabled = vscode.workspace.getConfiguration('git', editor.document.uri)
      .get<boolean>('blame.editorDecoration.enabled', false)
    const mode = resolveBlameMode(configuredMode, nativeBlameEnabled)
    const blameableLineCount = blameableDocumentLineCount(
      editor.document.lineCount,
      editor.document.lineAt(editor.document.lineCount - 1).text,
    )
    const window = mode === 'visibleLines'
      ? visibleBlameWindow(editor.visibleRanges.map((range) => ({
        startLine: range.start.line,
        endLine: range.end.line,
      })), blameableLineCount)
      : mode === 'activeLine'
        ? editor.selection.active.line < blameableLineCount
          ? { startLine: editor.selection.active.line + 1, endLine: editor.selection.active.line + 1 }
          : undefined
        : undefined
    if (!relativePath || !window) {
      editor.setDecorations(this.decoration, [])
      this.diagnose(!relativePath
        ? `[blame] skipped: file is outside repository ${session.root}`
        : `[blame] disabled mode=${mode} native=${nativeBlameEnabled}`)
      return
    }

    try {
      this.diagnose(`[blame] request path=${relativePath} lines=${window.startLine}-${window.endLine} mode=${mode}`)
      const result = await session.helper.blameLines({
        repositoryRoot: session.root,
        relativePath,
        startLine: window.startLine,
        endLine: window.endLine,
        contents: editor.document.isDirty ? editor.document.getText() : undefined,
        ignoreWhitespace: vscode.workspace.getConfiguration('gitgit.blame', editor.document.uri)
          .get<boolean>('ignoreWhitespace', false),
      }, this.cancellation.token)
      if (generation !== this.generation || editor !== vscode.window.activeTextEditor) return
      const decorations: vscode.DecorationOptions[] = []
      const reviewLinks = new Map<string, ReturnType<typeof reviewLinkForCommit>>()
      const now = new Date()
      for (const line of result.lines) {
        const index = line.line - 1
        if (index < 0 || index >= editor.document.lineCount) continue
        const displayDate = Number.isNaN(Date.parse(line.date))
          ? line.date
          : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(line.date))
        const workingTree = isWorkingTreeCommit(line.commit)
        const hover = new vscode.MarkdownString('$(git-commit) **GitGit Blame**\n\n', true)
        hover.appendMarkdown('**Subject**\n\n')
        hover.appendText(line.message || 'Uncommitted change')
        hover.appendMarkdown('\n\n')
        if (workingTree) {
          hover.appendText('State: Working tree')
        } else {
          hover.appendText(`Author: ${line.author.name}\nAuthored: ${displayDate}\nCommit: ${line.commit}`)
        }
        if (!workingTree && !reviewLinks.has(line.commit)) {
          const metadata = result.commitMetadata[line.commit]
          reviewLinks.set(line.commit, metadata
            ? reviewLinkForCommit(metadata.message, metadata.parentCount, session.webRemotes)
            : undefined)
        }
        const reviewLink = workingTree ? undefined : reviewLinks.get(line.commit)
        if (reviewLink) {
          hover.appendMarkdown(`\n\n[$(git-pull-request) ${reviewLink.label}](${reviewLink.url})`)
        }
        const end = editor.document.lineAt(index).range.end
        decorations.push({
          range: new vscode.Range(end, end),
          hoverMessage: hover,
          renderOptions: { after: { contentText: blameAnnotation(line.commit, line.author.name, line.date, now) } },
        })
      }
      editor.setDecorations(this.decoration, decorations)
      this.diagnose(`[blame] applied path=${relativePath} decorations=${decorations.length}`)
    } catch (error) {
      if (generation !== this.generation || isCancellation(error)) return
      editor.setDecorations(this.decoration, [])
      this.diagnose(`[blame] failed path=${relativePath}: ${errorMessage(error)}`)
    }
  }

  private diagnose(message: string): void {
    if (message === this.lastDiagnostic) return
    this.lastDiagnostic = message
    this.log(message)
  }
}

export class CommitHighlightController implements vscode.Disposable {
  private readonly changedDecoration = vscode.window.createTextEditorDecorationType({
    isWholeLine: true,
    backgroundColor: new vscode.ThemeColor('gitgit.commitChangeBackground'),
    overviewRulerColor: new vscode.ThemeColor('gitgit.commitChangeOverview'),
    overviewRulerLane: vscode.OverviewRulerLane.Left,
  })
  private readonly deletionDecoration = vscode.window.createTextEditorDecorationType({
    isWholeLine: true,
    borderStyle: 'solid',
    borderWidth: '1px 0 0 0',
    borderColor: new vscode.ThemeColor('gitgit.commitDeletionForeground'),
    overviewRulerColor: new vscode.ThemeColor('gitgit.commitDeletionForeground'),
    overviewRulerLane: vscode.OverviewRulerLane.Left,
  })
  private readonly deletedFileDecoration = vscode.window.createTextEditorDecorationType({
    isWholeLine: true,
    backgroundColor: new vscode.ThemeColor('gitgit.commitDeletedFileBackground'),
    overviewRulerColor: new vscode.ThemeColor('gitgit.commitDeletionForeground'),
    overviewRulerLane: vscode.OverviewRulerLane.Left,
  })
  private readonly subscriptions: vscode.Disposable[] = []
  private selection: CommitFileSelection | undefined
  private selectionRoot: string | undefined
  private prepared: { uri: string, diff: DiffFileResponse, kind: RevisionDocumentResource['kind'] } | undefined
  private loadCancellation: vscode.CancellationTokenSource | undefined
  private updateCancellation: vscode.CancellationTokenSource | undefined
  private loadGeneration = 0
  private updateGeneration = 0
  private readonly restoreGate = new RevisionRestoreGate()

  constructor(
    private readonly session: EditorRepositorySessionProvider,
    private readonly revisions: RevisionDocumentProvider,
    private readonly log: (message: string) => void = () => {},
  ) {
    this.subscriptions.push(
      this.changedDecoration,
      this.deletionDecoration,
      this.deletedFileDecoration,
      vscode.window.onDidChangeActiveTextEditor(() => { void this.update() }),
      vscode.workspace.onDidChangeTextDocument((event) => {
        if (event.document === vscode.window.activeTextEditor?.document
          && event.document.uri.scheme === 'file'
          && this.selection) this.clear()
      }),
    )
  }

  async select(selection: CommitFileSelection): Promise<void> {
    this.restoreGate.selectionStarted()
    const generation = ++this.loadGeneration
    this.loadCancellation?.cancel()
    this.loadCancellation?.dispose()
    this.loadCancellation = new vscode.CancellationTokenSource()
    const session = this.session()
    if (!session) {
      this.log('[highlight] selection ignored: repository session unavailable')
      this.clear()
      return
    }
    this.selection = undefined
    this.selectionRoot = undefined
    this.prepared = undefined
    this.clearDecorations()
    this.log(`[highlight] selected commit=${selection.commit.slice(0, 12)} path=${selection.path}`)
    try {
      const diff = await session.helper.diffFile({
        repositoryRoot: session.root,
        commit: selection.commit,
        path: selection.path,
        oldPath: selection.oldPath,
        context: 3,
      }, this.loadCancellation.token)
      let kind: RevisionDocumentResource['kind'] = 'target'
      let contentRevision = selection.commit
      let contentPath = selection.path
      let content: string
      try {
        content = (await session.helper.revisionContent({
          repositoryRoot: session.root,
          revision: contentRevision,
          relativePath: contentPath,
        }, this.loadCancellation.token)).content
      } catch (error) {
        if (!shouldUseDeletedPreimage(stableErrorCode(error), diff.parent, selection.status)) throw error
        kind = 'deleted-preimage'
        contentRevision = diff.parent
        contentPath = selection.oldPath ?? selection.path
        content = (await session.helper.revisionContent({
          repositoryRoot: session.root,
          revision: contentRevision,
          relativePath: contentPath,
        }, this.loadCancellation.token)).content
      }
      if (generation !== this.loadGeneration) return
      const uri = this.revisions.remember({
        v: 1,
        kind,
        root: session.root,
        contentRevision,
        contentPath,
        selectedCommit: selection.commit,
        selectedPath: selection.path,
        ...(selection.oldPath ? { selectedOldPath: selection.oldPath } : {}),
      }, content)
      this.selection = selection
      this.selectionRoot = session.root
      this.prepared = { uri: uri.toString(), diff, kind }
      const document = await openDocumentIfCurrent(
        () => vscode.workspace.openTextDocument(uri),
        async (opened) => { await vscode.window.showTextDocument(opened, { preview: true, preserveFocus: false }) },
        () => generation === this.loadGeneration && this.prepared?.uri === uri.toString(),
        (opened) => generation === this.loadGeneration && vscode.window.activeTextEditor?.document === opened,
      )
      if (!document) return
      await this.update()
    } catch (error) {
      if (generation !== this.loadGeneration || isCancellation(error)) return
      this.clear()
      const message = errorMessage(error)
      this.log(`[highlight] failed to open revision commit=${selection.commit.slice(0, 12)} path=${selection.path}: ${message}`)
      void vscode.window.showErrorMessage(`GitGit: ${message}`)
    }
  }

  async restore(resource: RevisionDocumentResource, uri: vscode.Uri): Promise<void> {
    if (!this.restoreGate.allows(uri.toString())) {
      this.clearDecorations()
      this.log('[highlight] restore suppressed after explicit clear')
      return
    }
    const session = this.session()
    if (!session || session.root !== resource.root) {
      this.log('[highlight] restore ignored: repository session does not match revision root')
      this.clear()
      return
    }
    const selection = revisionSelection(resource)
    if (this.prepared?.uri === uri.toString()
      && this.selectionRoot === session.root
      && this.selection
      && revisionResourceMatchesSelection(resource, session.root, this.selection)) {
      await this.update()
      return
    }

    const generation = ++this.loadGeneration
    this.loadCancellation?.cancel()
    this.loadCancellation?.dispose()
    this.loadCancellation = new vscode.CancellationTokenSource()
    this.selection = selection
    this.selectionRoot = session.root
    this.prepared = undefined
    this.clearDecorations()
    try {
      const diff = await session.helper.diffFile({
        repositoryRoot: session.root,
        commit: selection.commit,
        path: selection.path,
        oldPath: selection.oldPath,
        context: 3,
      }, this.loadCancellation.token)
      if (generation !== this.loadGeneration || uri.toString() !== vscode.window.activeTextEditor?.document.uri.toString()) return
      this.prepared = { uri: uri.toString(), diff, kind: resource.kind }
      this.log(`[highlight] restored revision commit=${selection.commit.slice(0, 12)} path=${selection.path}`)
      await this.update()
    } catch (error) {
      if (generation !== this.loadGeneration || isCancellation(error)) return
      this.clear()
      this.log(`[highlight] failed to restore revision commit=${selection.commit.slice(0, 12)} path=${selection.path}: ${errorMessage(error)}`)
    }
  }

  clear(): void {
    this.selection = undefined
    this.selectionRoot = undefined
    this.prepared = undefined
    this.loadGeneration++
    this.updateGeneration++
    this.loadCancellation?.cancel()
    this.updateCancellation?.cancel()
    this.clearDecorations()
  }

  clearByUser(activeUri?: vscode.Uri): void {
    const revisionUri = activeUri?.scheme === REVISION_DOCUMENT_SCHEME
      ? activeUri.toString()
      : this.prepared?.uri
    this.restoreGate.suppress(revisionUri)
    this.clear()
  }

  refresh(): void {
    void this.update()
  }

  dispose(): void {
    this.clear()
    this.loadCancellation?.dispose()
    this.updateCancellation?.dispose()
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private async update(): Promise<void> {
    const generation = ++this.updateGeneration
    this.updateCancellation?.cancel()
    this.updateCancellation?.dispose()
    this.updateCancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    const selection = this.selection
    if (!editor || !session || !selection) {
      this.clearDecorations()
      if (selection) {
        this.log(!session
          ? '[highlight] skipped: repository session unavailable'
          : !editor
            ? '[highlight] skipped: no active editor'
            : '[highlight] skipped: selection unavailable')
      }
      return
    }
    if (this.selectionRoot !== session.root) {
      this.log(`[highlight] cleared after repository switch from ${this.selectionRoot ?? '<unknown>'} to ${session.root}`)
      this.clear()
      return
    }
    const prepared = this.prepared
    if (prepared
      && editor.document.uri.scheme === REVISION_DOCUMENT_SCHEME
      && editor.document.uri.toString() !== prepared.uri) {
      this.clearDecorations()
      this.log(`[highlight] skipped: active document is not the selected revision path=${selection.path}`)
      return
    }

    try {
      const resource = this.revisions.resource(editor.document.uri)
      let kind: RevisionDocumentResource['kind'] = 'target'
      if (editor.document.uri.scheme === REVISION_DOCUMENT_SCHEME) {
        if (!resource || !revisionResourceMatchesSelection(resource, session.root, selection)) {
          this.clearDecorations()
          this.log('[highlight] skipped: revision document does not match the active selection')
          return
        }
        kind = resource.kind
      } else if (editor.document.uri.scheme === 'file') {
        const relativePath = repositoryRelativePath(session.root, editor.document.uri.fsPath)
        if (relativePath !== selection.path) {
          this.clearDecorations()
          this.log(`[highlight] skipped: active path=${relativePath ?? '<outside repository>'} selected path=${selection.path}`)
          return
        }
        const revision = await session.helper.revisionContent({
          repositoryRoot: session.root,
          revision: selection.commit,
          relativePath: selection.path,
        }, this.updateCancellation.token)
        if (generation !== this.updateGeneration || editor !== vscode.window.activeTextEditor) return
        if (!revisionContentMatchesDocument(revision.content, editor.document.getText())) {
          this.clearDecorations()
          this.log(`[highlight] skipped: working-tree content differs from commit=${selection.commit.slice(0, 12)} path=${selection.path}`)
          return
        }
      } else {
        this.clearDecorations()
        this.log(`[highlight] skipped: unsupported document scheme ${editor.document.uri.scheme}`)
        return
      }

      const result = prepared?.diff ?? await session.helper.diffFile({
        repositoryRoot: session.root,
        commit: selection.commit,
        path: selection.path,
        oldPath: selection.oldPath,
        context: 3,
      }, this.updateCancellation.token)
      if (generation !== this.updateGeneration || editor !== vscode.window.activeTextEditor) return
      this.applyDecorations(editor, result, kind, selection)
    } catch (error) {
      if (generation !== this.updateGeneration || isCancellation(error)) return
      this.clearDecorations()
      this.log(`[highlight] failed commit=${selection.commit.slice(0, 12)} path=${selection.path}: ${errorMessage(error)}`)
    }
  }

  private applyDecorations(
    editor: vscode.TextEditor,
    result: DiffFileResponse,
    kind: RevisionDocumentResource['kind'],
    selection: CommitFileSelection,
  ): void {
    const changed = kind === 'target'
      ? normalizedTargetRanges(result.targetRanges, editor.document.lineCount).map((range) => ({
        range: new vscode.Range(
          new vscode.Position(range.startLine - 1, 0),
          editor.document.lineAt(range.endLine - 1).range.end,
        ),
        hoverMessage: `Changed by ${selection.commit.slice(0, 12)}`,
      }))
      : []
    const deletions = kind === 'target'
      ? normalizedDeletionAnchors(result.deletionAnchors, editor.document.lineCount).map((anchor) => ({
        range: editor.document.lineAt(anchor.anchorLine - 1).range,
        hoverMessage: `Deletion in ${selection.commit.slice(0, 12)}`,
      }))
      : []
    const actualLineCount = blameableDocumentLineCount(
      editor.document.lineCount,
      editor.document.lineAt(editor.document.lineCount - 1).text,
    )
    const deletedFile = kind === 'deleted-preimage' && actualLineCount > 0
      ? [{
        range: new vscode.Range(
          new vscode.Position(0, 0),
          editor.document.lineAt(actualLineCount - 1).range.end,
        ),
        hoverMessage: `File deleted by ${selection.commit.slice(0, 12)}`,
      }]
      : []
    editor.setDecorations(this.changedDecoration, changed)
    editor.setDecorations(this.deletionDecoration, deletions)
    editor.setDecorations(this.deletedFileDecoration, deletedFile)
    this.log(`[highlight] applied revision commit=${selection.commit.slice(0, 12)} path=${selection.path} ranges=${changed.length} deletions=${deletions.length} deletedFile=${deletedFile.length}`)
  }

  private clearDecorations(): void {
    for (const editor of vscode.window.visibleTextEditors) {
      editor.setDecorations(this.changedDecoration, [])
      editor.setDecorations(this.deletionDecoration, [])
      editor.setDecorations(this.deletedFileDecoration, [])
    }
  }
}

class HistoryItem extends vscode.TreeItem {
  constructor(commit: HistoryCommit, selection: CommitFileSelection, now = new Date()) {
    const subject = commit.message.split('\n')[0] || commit.shortCommit
    super(subject, vscode.TreeItemCollapsibleState.None)
    this.description = fileHistoryDescription(commit.shortCommit, commit.author.name, commit.date, now)
    const tooltip = new vscode.MarkdownString(undefined, false)
    tooltip.appendMarkdown('**Commit message**\n\n')
    tooltip.appendText(commit.message)
    tooltip.appendMarkdown('\n\n')
    tooltip.appendText(`${commit.author.name} <${commit.author.email ?? ''}>\n${commit.date}\n${commit.commit}`)
    this.tooltip = tooltip
    this.iconPath = new vscode.ThemeIcon('git-commit')
    this.command = {
      command: 'gitgit.commit.select',
      title: 'Open Commit Revision',
      arguments: [selection],
    }
    this.contextValue = 'gitgit.historyCommit'
    this.accessibilityInformation = {
      label: fileHistoryAccessibilityLabel(subject, commit.shortCommit, commit.author.name, commit.date, now),
      role: 'treeitem',
    }
  }
}

class StatusItem extends vscode.TreeItem {
  constructor(label: string, description?: string, icon = 'info') {
    super(label, vscode.TreeItemCollapsibleState.None)
    this.description = description
    this.iconPath = new vscode.ThemeIcon(icon)
    this.tooltip = [label, description].filter(Boolean).join('\n')
    this.accessibilityInformation = {
      label: [label, description].filter(Boolean).join('. '),
      role: 'treeitem',
    }
  }
}

export class FileHistoryProvider implements vscode.TreeDataProvider<vscode.TreeItem>, vscode.Disposable {
  private readonly changed = new vscode.EventEmitter<vscode.TreeItem | undefined>()
  private cancellation: vscode.CancellationTokenSource | undefined
  private generation = 0
  readonly onDidChangeTreeData = this.changed.event

  constructor(
    private readonly session: EditorRepositorySessionProvider,
    private readonly revisions: Pick<RevisionDocumentProvider, 'resource'>,
  ) {}

  getTreeItem(element: vscode.TreeItem): vscode.TreeItem {
    return element
  }

  async getChildren(element?: vscode.TreeItem): Promise<vscode.TreeItem[]> {
    if (element) return []
    const generation = ++this.generation
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.cancellation = new vscode.CancellationTokenSource()

    const editor = vscode.window.activeTextEditor
    const session = this.session()
    if (!session) return [new StatusItem('No Git repository discovered')]
    if (!editor) return [new StatusItem('Open a local file to inspect its history')]
    const revision = this.revisions.resource(editor.document.uri)
    const relativePath = editor.document.uri.scheme === 'file'
      ? repositoryRelativePath(session.root, editor.document.uri.fsPath)
      : revision?.root === session.root ? revision.selectedPath : undefined
    if (!relativePath) return [new StatusItem('Active file is outside this repository', undefined, 'warning')]

    try {
      const result = await session.helper.historyFile({
        repositoryRoot: session.root,
        relativePath,
        revision: revision?.selectedCommit,
        limit: 100,
      }, this.cancellation.token)
      if (generation !== this.generation) return []
      if (result.commits.length === 0) return [new StatusItem('No commits found for this file')]
      const selections = traceFileHistorySelections(result.commits, relativePath)
      const now = new Date()
      return result.commits.map((commit, index) => new HistoryItem(commit, selections[index] ?? {
        commit: commit.commit,
        path: relativePath,
      }, now))
    } catch (error) {
      if (generation !== this.generation || isCancellation(error)) return []
      return [new StatusItem('Unable to read file history', errorMessage(error), 'error')]
    }
  }

  refresh(): void {
    this.generation++
    this.cancellation?.cancel()
    this.changed.fire(undefined)
  }

  dispose(): void {
    this.generation++
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.changed.dispose()
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

function stableErrorCode(error: unknown): string {
  return error instanceof Error && 'stableCode' in error ? String(error.stableCode) : ''
}

function revisionResourceMatchesSelection(
  resource: RevisionDocumentResource,
  root: string,
  selection: CommitFileSelection,
): boolean {
  return resource.root === root
    && resource.selectedCommit === selection.commit
    && resource.selectedPath === selection.path
    && resource.selectedOldPath === selection.oldPath
}

function isCancellation(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  const stableCode = 'stableCode' in error ? String(error.stableCode) : ''
  return stableCode === 'request_cancelled' || /cancel/iu.test(error.message)
}
