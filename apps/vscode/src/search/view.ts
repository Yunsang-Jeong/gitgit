import { randomBytes } from 'node:crypto'

import * as vscode from 'vscode'

import {
  buildSearchRequest,
  initialSearchState,
  reduceSearchState,
  validateSearchDraft,
  type SearchDraft,
  type SearchViewState,
} from './model.js'
import type { SearchRequest, SearchResponse, SearchResult } from '../helper/protocol.js'
import {
  contentSecurityPolicy,
  escapeHtml,
  parseWebviewMessage,
  type SearchResultSelection,
} from './webview.js'

export type { SearchResultSelection } from './webview.js'

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


export class SearchViewProvider implements vscode.WebviewViewProvider, vscode.Disposable {
  private view: vscode.WebviewView | undefined
  private searchState: SearchViewState = initialSearchState()
  private cancellation: vscode.CancellationTokenSource | undefined
  private nextRequestId = 1
  private repositoryRoot: string | undefined
  private readonly subscriptions: vscode.Disposable[] = []
  private readonly selected = new vscode.EventEmitter<SearchResultSelection>()
  readonly onDidSelectResult = this.selected.event

  constructor(private readonly runtime: SearchRuntime) {
    this.repositoryRoot = runtime.state.repository?.root
    this.subscriptions.push(runtime.onDidChange(() => this.runtimeChanged()))
  }

  resolveWebviewView(view: vscode.WebviewView): void {
    this.view = view
    view.webview.options = { enableScripts: true }
    const messages = view.webview.onDidReceiveMessage((raw) => this.onMessage(raw))
    view.onDidDispose(() => {
      messages.dispose()
      this.cancelActiveSearch()
      if (this.view === view) this.view = undefined
    })
    this.render()
  }

  dispose(): void {
    this.cancellation?.cancel()
    this.cancellation?.dispose()
    this.selected.dispose()
    for (const subscription of this.subscriptions) subscription.dispose()
  }

  private async onMessage(raw: unknown): Promise<void> {
    const message = parseWebviewMessage(raw)
    if (!message) return
    if ('action' in message) {
      this.selected.fire(message)
      return
    }
    if (message.type === 'draftChanged') {
      this.searchState = reduceSearchState(this.searchState, { type: 'edit', draft: message.draft })
      this.postState()
      return
    }
    if (message.type === 'cancel') {
      const requestId = this.searchState.activeRequestId
      if (requestId === undefined) return
      this.cancellation?.cancel()
      this.cancellation?.dispose()
      this.cancellation = undefined
      this.searchState = reduceSearchState(this.searchState, { type: 'cancel', requestId })
      this.render()
      return
    }
    await this.search(message.draft)
  }

  private async search(draft: SearchDraft): Promise<void> {
    const validated = validateSearchDraft(draft)
    this.searchState = reduceSearchState(this.searchState, { type: 'edit', draft })
    if (validated.error) {
      this.searchState = { ...this.searchState, error: validated.error, phase: 'error' }
      this.render()
      return
    }
    const repository = this.runtime.state.repository
    if (!repository || this.runtime.state.phase !== 'ready') {
      this.searchState = { ...this.searchState, error: this.runtime.state.message, phase: 'error' }
      this.render()
      return
    }

    this.cancellation?.cancel()
    this.cancellation?.dispose()
    const cancellation = new vscode.CancellationTokenSource()
    this.cancellation = cancellation
    const requestId = this.nextRequestId++
    const request = buildSearchRequest(repository.root, requestId, draft, validated.patterns)
    this.searchState = reduceSearchState(this.searchState, { type: 'start', requestId, draft })
    this.render()

    try {
      const response = await this.runtime.runSearch(
        request,
        (progress) => {
          this.searchState = reduceSearchState(this.searchState, { type: 'progress', progress })
          this.postState()
        },
        cancellation.token,
      )
      this.searchState = reduceSearchState(this.searchState, { type: 'success', requestId, draft, response })
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
      this.render()
    }
  }

  private render(): void {
    if (!this.view) return
    const nonce = nonceValue()
    this.view.webview.html = searchHtml(this.runtime.state, this.searchState, nonce)
  }

  private postState(): void {
    if (!this.view) return
    const validated = validateSearchDraft(this.searchState.draft)
    const status = this.runtime.state.phase === 'ready'
      ? this.searchState.error || validated.error || this.searchState.notice || searchStatus(this.searchState)
      : this.runtime.state.message
    void this.view.webview.postMessage({
      type: 'searchState',
      status,
      searchDisabled: this.runtime.state.phase !== 'ready'
        || this.searchState.phase === 'running'
        || Boolean(validated.error),
      cancelDisabled: this.searchState.phase !== 'running',
    })
  }

  private runtimeChanged(): void {
    const nextRoot = this.runtime.state.phase === 'ready' ? this.runtime.state.repository?.root : undefined
    if (nextRoot && this.repositoryRoot && nextRoot !== this.repositoryRoot) {
      const draft = this.searchState.draft
      this.cancelActiveSearch()
      this.searchState = { ...initialSearchState(), draft }
    } else if (this.runtime.state.phase !== 'ready' && this.runtime.state.phase !== 'loading') {
      const draft = this.searchState.draft
      this.cancelActiveSearch()
      this.searchState = { ...initialSearchState(), draft }
      this.repositoryRoot = undefined
    }
    if (nextRoot) this.repositoryRoot = nextRoot
    this.render()
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
}

function nonceValue(): string {
  return randomBytes(24).toString('hex')
}

export function searchHtml(runtime: SearchRuntime['state'], state: SearchViewState, nonce: string): string {
  const validated = validateSearchDraft(state.draft)
  const disabled = runtime.phase !== 'ready' || state.phase === 'running' || Boolean(validated.error)
  const status = runtime.phase === 'ready'
    ? state.error || validated.error || state.notice || searchStatus(state)
    : runtime.message
  const results = groupResults(state.results).map(renderResult).join('')
    || '<p class="empty">No successful Search results yet.</p>'
  const initial = JSON.stringify(state.draft).replaceAll('<', '\\u003c')
  const policy = contentSecurityPolicy(nonce)

  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="${policy}">
<style nonce="${nonce}">:root{color-scheme:light dark}body{box-sizing:border-box;padding:0 12px 16px;color:var(--vscode-foreground);font-family:var(--vscode-font-family);font-size:var(--vscode-font-size)}*,*:before,*:after{box-sizing:inherit}.section{margin:14px 0 6px;font-size:11px;font-weight:700;letter-spacing:.08em;color:var(--vscode-descriptionForeground)}input,select,button{font:inherit;color:var(--vscode-input-foreground);background:var(--vscode-input-background);border:1px solid var(--vscode-input-border,transparent);min-height:28px}input,select{width:100%;padding:4px 7px}.grid{display:grid;grid-template-columns:1fr 1fr;gap:6px}.wide{grid-column:1/-1}.checks{display:flex;flex-wrap:wrap;gap:10px;margin:7px 0}.checks label{display:flex;gap:5px;align-items:center}.checks input{width:auto;min-height:auto}.actions{display:flex;gap:6px;margin-top:8px}.actions button{padding:4px 10px;background:var(--vscode-button-background);color:var(--vscode-button-foreground);border:0}.actions button:hover{background:var(--vscode-button-hoverBackground)}.actions button:disabled{opacity:.55}.actions .secondary{background:var(--vscode-button-secondaryBackground);color:var(--vscode-button-secondaryForeground)}.status{min-height:34px;margin:8px 0;padding:7px;border-left:2px solid var(--vscode-focusBorder);color:var(--vscode-descriptionForeground);word-break:break-word}.repo{word-break:break-all;color:var(--vscode-descriptionForeground)}.result{padding:9px 0;border-top:1px solid var(--vscode-panel-border)}.headline{display:flex;gap:6px;align-items:center}.commit{font-family:var(--vscode-editor-font-family);color:var(--vscode-textLink-foreground)}.message{font-weight:600;overflow-wrap:anywhere}.meta,.files{margin-top:3px;color:var(--vscode-descriptionForeground);font-size:12px}.badges{display:flex;flex-wrap:wrap;gap:4px;margin-top:5px}.badge{padding:1px 5px;border:1px solid var(--vscode-badge-background);border-radius:7px}.file{display:flex;gap:5px;align-items:center;margin-top:5px}.file span{flex:1;overflow-wrap:anywhere}.file button{min-height:22px;padding:1px 5px;background:transparent;color:var(--vscode-textLink-foreground);border-color:var(--vscode-textLink-foreground)}.empty{color:var(--vscode-descriptionForeground)}</style></head><body>
<div class="section">PRESET</div><p class="repo">${escapeHtml(runtime.repository?.root ?? runtime.message)}</p>
<div class="section">FILTERS</div><div class="grid"><label class="wide">Engine<select id="engine"><option value="glob">Glob</option><option value="regex">Regex</option></select></label><label>Author<input id="author" autocomplete="off"></label><label>Since<input id="since" autocomplete="off" placeholder="last:30d"></label><label>Until<input id="until" autocomplete="off"></label></div><div class="checks"><label><input id="allRefs" type="checkbox">All refs</label><label><input id="followRename" type="checkbox">Follow renames</label></div>
<div class="section">SEARCH</div><label>Expression<input id="expression" autocomplete="off" spellcheck="false" placeholder="MSG: *cache* OR FILE: **/*.go"></label><div class="actions"><button id="search" ${disabled ? 'disabled' : ''}>Search</button><button id="cancel" class="secondary" ${state.phase === 'running' ? '' : 'disabled'}>Cancel</button></div><div id="status" class="status" role="status" aria-live="polite">${escapeHtml(status)}</div><div aria-label="Search results">${results}</div>
<script nonce="${nonce}">const vscode=acquireVsCodeApi();const initial=${initial};const ids=['expression','engine','author','since','until','allRefs','followRename'];for(const id of ids){const el=document.getElementById(id);el[id==='allRefs'||id==='followRename'?'checked':'value']=initial[id]??false;el.addEventListener('input',()=>{document.getElementById('status').textContent='Validating filters…';vscode.postMessage({type:'draftChanged',draft:readDraft()});});}function readDraft(){return {expression:document.getElementById('expression').value,engine:document.getElementById('engine').value,author:document.getElementById('author').value,since:document.getElementById('since').value,until:document.getElementById('until').value,allRefs:document.getElementById('allRefs').checked,followRename:document.getElementById('followRename').checked};}window.addEventListener('message',(event)=>{const state=event.data;if(!state||state.type!=='searchState')return;document.getElementById('status').textContent=state.status;document.getElementById('search').disabled=state.searchDisabled;document.getElementById('cancel').disabled=state.cancelDisabled;});document.getElementById('search').addEventListener('click',()=>vscode.postMessage({type:'search',draft:readDraft()}));document.getElementById('cancel').addEventListener('click',()=>vscode.postMessage({type:'cancel'}));document.addEventListener('click',(event)=>{const target=event.target;if(!(target instanceof Element))return;const button=target.closest('button[data-action]');if(!button)return;vscode.postMessage({type:button.dataset.action,commit:button.dataset.commit,path:button.dataset.path,oldPath:button.dataset.oldPath||undefined});});</script></body></html>`
}

function searchStatus(state: SearchViewState): string {
  if (state.phase === 'running') return state.progress
    ? `Searching ${state.progress.scanned} / ${state.progress.total} commits…`
    : 'Starting Search…'
  if (state.phase === 'ready') return `${state.results.length} file matches · ${state.scanned} commits scanned.`
  if (state.phase === 'stale') return 'Expression changed. Run Search to refresh these results.'
  return 'Enter MSG:, DIFF:, or FILE: conditions, then choose Search.'
}

function groupResults(results: SearchResult[]): SearchResult[] {
  const grouped = new Map<string, SearchResult>()
  for (const result of results) {
    const current = grouped.get(result.commit)
    if (!current) {
      grouped.set(result.commit, { ...result, refs: [...result.refs], files: [...result.files], matchSources: [...result.matchSources] })
      continue
    }
    for (const source of result.matchSources) if (!current.matchSources.includes(source)) current.matchSources.push(source)
    for (const file of result.files) {
      if (!current.files.some((candidate) => candidate.path === file.path && candidate.oldPath === file.oldPath)) current.files.push(file)
    }
  }
  return [...grouped.values()]
}

function renderResult(result: SearchResult): string {
  const files = result.files.map((file) => `<div class="file"><span>${escapeHtml(file.status)} ${escapeHtml(file.path)}</span><button data-action="selectCommitFile" data-commit="${escapeHtml(result.commit)}" data-path="${escapeHtml(file.path)}" data-old-path="${escapeHtml(file.oldPath ?? '')}">Select</button><button data-action="openFile" data-commit="${escapeHtml(result.commit)}" data-path="${escapeHtml(file.path)}" data-old-path="${escapeHtml(file.oldPath ?? '')}">Open</button></div>`).join('')
  const badges = result.matchSources.map((source) => `<span class="badge">${escapeHtml(source === 'msg' ? 'Message' : source.toUpperCase())}</span>`).join('')
  return `<article class="result"><div class="headline"><span class="commit">${escapeHtml(result.shortCommit)}</span><span class="message">${escapeHtml(result.message)}</span></div><div class="meta">${escapeHtml(result.author.name)} · ${escapeHtml(result.date)}</div><div class="badges">${badges}</div><div class="files">${files}</div></article>`
}
