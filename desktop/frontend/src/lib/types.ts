export type PatternSource = 'msg' | 'diff' | 'file'
export type SearchPatternJoin = 'and' | 'or'

export interface Pattern {
  source: PatternSource
  value: string
  join?: SearchPatternJoin
  open_groups?: number
  close_groups?: number
}

export interface Author {
  name: string
  email: string
}

export interface FileChange {
  status: string
  old_path?: string
  path: string
}

export interface SparseState {
  enabled: boolean
  cone: boolean
  directories?: string[]
}

export interface WorktreeInfo {
  path: string
  head: string
  branch?: string
  detached: boolean
  bare: boolean
  locked: boolean
  prunable: boolean
  dirty: boolean
  merged_into_default: boolean
  sparse: SparseState
}

export interface RepositoryState {
  root: string
  project_root: string
  name: string
  branch: string
  default_branch: string
  user: Author
  upstream?: string
  head?: string
  dirty: boolean
  ahead: number
  behind: number
  worktrees: WorktreeInfo[]
  remotes: RemoteInfo[]
}

export interface RemoteInfo {
  name: string
  url: string
}

export interface RemoteSyncResult {
  state: RepositoryState
  warnings?: string[]
}

export interface RegisteredProject {
  root: string
  name: string
  favorite: boolean
}

export interface ProjectDiscoveryResult {
  directory: string
  found: number
  added: number
  canceled: boolean
  projects: RegisteredProject[]
}

export interface AppSettings {
  history_batch_size: number
  ide: IDEPreference
  terminal: TerminalPreference
  changed_files_view: ChangedFilesView
  filter_logic: CommitFilterLogic
  presets: CommitFilterPreset[]
  remote_badges: RemoteBadgeRule[]
}

export interface RemoteBadgeRule {
  id: string
  pattern: string
  icon: string
}

export type IDEPreference = 'vscode' | 'cursor' | 'zed' | 'idea' | 'xcode'
export type TerminalPreference = 'terminal' | 'iterm2' | 'warp' | 'ghostty' | 'wezterm'
export type ChangedFilesView = 'list' | 'tree'

export type NavigatorView = 'commit' | 'worktrees' | 'search'

export interface SearchRequest {
  request_id: number
  patterns: Pattern[]
  engine: string
  scope: string
  all_refs: boolean
  author: string
  since: string
  until: string
  follow_rename: boolean
  limit: number
  context: number
}

export interface SearchProgress {
  request_id: number
  scanned: number
  total: number
}

export interface SearchResult {
  author: Author
  commit: string
  short_commit: string
  message: string
  date: string
  refs?: string[]
  file: FileChange
  files: FileChange[]
  diff: string
  match_sources: PatternSource[]
  matched_files?: FileChange[]
}

export type SearchSessionStatus = 'running' | 'ready' | 'stale' | 'error'

export interface SearchSessionSummary {
  id: string
  title: string
  project: string
  query: string
  status?: SearchSessionStatus
  result_count: number
  last_searched_at?: string
}

export interface SearchResponse {
  scope: string
  all_refs: boolean
  scanned: number
  count: number
  results: SearchResult[]
}

export interface HistoryRequest {
  scope: string
  all_branches: boolean
  related_scope: string
  limit: number
  skip: number
}

export interface CommitSummary {
  author: Author
  commit: string
  short_commit: string
  parents: string[]
  message: string
  date: string
  refs?: string[]
  branches?: string[]
  historical_branch?: string
  files: FileChange[]
}

export interface HistoryResponse {
  scope: string
  all_branches: boolean
  total: number
  branch_point?: string
  branches: string[]
  commits: CommitSummary[]
}

export interface HistoryBranchesResponse {
  branches: Record<string, string[]>
}

export interface RepositoryTreeEntry {
  name: string
  path: string
  object_type: 'tree' | 'blob' | 'commit' | string
  oid: string
}

export interface RepositoryTreeResponse {
  revision: string
  directory: string
  entries: RepositoryTreeEntry[]
}

export interface CommitDetail extends CommitSummary {
  file: FileChange
  diff: string
}

export interface CommitEditCommit {
  author: Author
  commit: string
  short_commit: string
  message: string
  date: string
  files: FileChange[]
}

export interface CommitEditStack {
  branch: string
  default_branch: string
  head: string
  base: string
  default_branch_target: boolean
  commits: CommitEditCommit[]
}

export interface CommitFileContent {
  commit: string
  path: string
  content: string
  exists: boolean
  editable: boolean
  reason?: string
}

export interface CommitFileEdit {
  path: string
  content: string
  delete: boolean
}

export interface RewriteCommit {
  commit: string
  message: string
  file_edits?: CommitFileEdit[]
}

export interface RewriteCommitsRequest {
  branch: string
  expected_head: string
  base: string
  confirm_default_branch: boolean
  commits: RewriteCommit[]
}

export interface RewriteCommitsResponse {
  state: RepositoryState
  head: string
  backup_ref: string
  warning?: string
}

export type CommitFilterField = 'branch' | 'author' | 'message' | 'file' | 'date'
export type CommitFilterAction = 'hide' | 'show'
export type CommitFilterJoin = 'and' | 'or'

export interface CommitFilterLogic {
  show: CommitFilterJoin
  hide: CommitFilterJoin
}

export interface CommitFilterRule {
  id: string
  field: CommitFilterField
  action: CommitFilterAction
  pattern: string
}

export interface CommitFilterPreset {
  id: string
  label: string
  rules: CommitFilterRule[]
}

export interface HistoryFilterProgress {
  presets: string[]
  conditions: string[]
  scope: string
  target: number
  visible: number
  scanned: number
  total: number
}

export interface ContextMenuItem {
  label: string
  shortcut?: string
  danger?: boolean
  disabled?: boolean
  separatorBefore?: boolean
  run: () => void | Promise<void>
}
