<script lang="ts">
  import { onMount, tick } from 'svelte'
  import CommitEditor from './components/CommitEditor.svelte'
  import CommitTable from './components/CommitTable.svelte'
  import HistoryToolbar from './components/HistoryToolbar.svelte'
  import Inspector from './components/Inspector.svelte'
  import SearchWorkspace from './components/SearchWorkspace.svelte'
  import SettingsModal from './components/SettingsModal.svelte'
  import StatusBar from './components/StatusBar.svelte'
  import Topbar from './components/Topbar.svelte'
  import WorktreeGrid from './components/WorktreeGrid.svelte'
  import { api } from './lib/api'
  import { worktreeHistoryScope } from './lib/branch-options'
  import { normalizeSearchBoundary } from './lib/datetime'
  import { visibleCommits } from './lib/history'
  import { cloneFilterPresets, defaultFilterLogic, defaultFilterPresets, resolvePresetRules } from './lib/presets'
  import { defaultRemoteBadgeRules, normalizeRemoteBadgeIcon } from './lib/remotes'
  import { groupSearchResultsByCommit, searchResultCommitCount } from './lib/search-results'
  import { searchExpressionError, searchPatternText } from './lib/search-expression'
  import type {
    AppSettings,
    ChangedFilesView,
    CommitDetail,
    CommitEditStack,
    CommitFilterRule,
    CommitFilterAction,
    CommitFilterField,
    CommitFilterLogic,
    CommitFilterPreset,
    CommitSummary,
    CommitFileContent,
    HistoryResponse,
    HistoryFilterProgress,
    IDEPreference,
    NavigatorView,
    Pattern,
    RegisteredProject,
    RemoteBadgeRule,
    RepositoryState,
    RepositoryTreeResponse,
    RewriteCommitsRequest,
    SearchRequest,
    SearchProgress,
    SearchResult,
    SearchSessionStatus,
    TerminalPreference,
    WorktreeInfo,
  } from './lib/types'

  type SearchDraftSnapshot = {
    patterns: Pattern[]
    engine: string
    scope: string
    allRefs: boolean
    author: string
    since: string
    until: string
  }

  type SearchSessionState = {
    id: string
    title: string
    projectRoot: string
    worktreeRoot: string
    draft: SearchDraftSnapshot
    results: SearchResult[]
    selectedIndex: number
    scanned: number
    resultScope: string
    resultAllRefs: boolean
    executedDraft: SearchDraftSnapshot | null
    hasSearched: boolean
    lastSearchedAt?: string
    error: string
  }

  type StatusKind = 'info' | 'success' | 'warning' | 'error'

  const paneWidthKey = 'gitgit.pane-widths.v1'
  const settingsKey = 'gitgit.settings.v1'
  const commitRowHeight = 50

  let repository: RepositoryState | null = null
  let appVersion = 'dev'
  let projects: RegisteredProject[] = []
  let activeProjectRoot = ''
  let history: HistoryResponse = { scope: 'All branches', all_branches: true, total: 0, branches: [], commits: [] }
  let historyScope = 'HEAD'
  let historyAllBranches = true
  let historyLoading = false
  let historyLoadingMore = false
  let historyLoadMorePromise: Promise<boolean> | null = null
  let historyAutoLoadEnabled = true
  let branchMembershipLoading = false
  let selectedCommit = ''
  let historyDetail: CommitDetail | null = null
  let detailOverride: CommitDetail | null = null
  let filterRules: CommitFilterRule[] = []
  let activePresetIDs: string[] = []
  let historyFilterProgress: HistoryFilterProgress | null = null

  let patterns: Pattern[] = []
  let engine = 'glob'
  let scope = 'HEAD'
  let searchAllRefs = false
  let author = ''
  let since = ''
  let until = ''
  let results: SearchResult[] = []
  let selectedSearchIndex = -1
  let searching = false
  let searchProgress: SearchProgress | null = null
  let hasSearched = false
  let searchLastSearchedAt = ''
  let scanned = 0
  let resultScope = 'HEAD'
  let resultAllRefs = false
  let executedSearchDraft: SearchDraftSnapshot | null = null
  let searchError = ''
  let searchSessions: SearchSessionState[] = []
  let activeSearchSessionID = ''
  let searchSessionSequence = 0

  let refreshing = false
  let syncing = false
  let statusMessage = ''
  let statusKind: StatusKind = 'info'
  let settingsOpen = false
  let commitEditorOpen = false
  let commitEditorStack: CommitEditStack | null = null
  let commitEditorLoading = false
  let commitEditorApplying = false
  let commitEditorError = ''
  let discoveringProjects = false
  let discoveryMessage = ''
  let appSettings: AppSettings = defaultAppSettings()
  let navigatorView: NavigatorView = 'commit'
  let inspectorWidth = 440
  let removingWorktrees = false
  let projectSwitching = false
  let worktreeSwitching = false
  let repositoryTransitioning = false
  let currentWorktreeDetached = false
  let inspectorFileRevision = ''
  let repositoryRequestID = 0
  let historyRequestID = 0
  let detailRequestID = 0
  let branchRequestID = 0
  let filterRequestID = 0
  let searchRequestID = 0
  let syncRequestID = 0
  let commitEditorRequestID = 0

  $: filterRules = resolvePresetRules(appSettings.presets, activePresetIDs, repository?.user ?? { name: '', email: '' })
  $: commitFilterRules = branchMembershipLoading ? filterRules.filter((rule) => rule.field !== 'branch') : filterRules
  $: filteredCommits = visibleCommits(history.commits, commitFilterRules, appSettings.filter_logic)
  $: groupedSearchResults = groupSearchResultsByCommit(results)
  $: selectedSearchResult = groupedSearchResults[selectedSearchIndex] ?? null
  $: selectedForInspector = detailOverride ?? historyDetail
  $: inspectorFileRevision = repository ? (historyAllBranches ? repository.default_branch : historyScope) : ''
  $: repositoryTransitioning = projectSwitching || worktreeSwitching
  $: currentWorktreeDetached = Boolean(repository && (repository.worktrees.find((worktree) => worktree.path === repository?.root)?.detached ?? repository.branch === 'detached'))
  $: canEditCommits = Boolean(
    repository
    && selectedCommit
    && !currentWorktreeDetached
    && !historyAllBranches
    && historyScope === repository.branch
    && !repositoryTransitioning,
  )
  $: editDisabledReason = commitEditDisabledReason()
  $: searchDraft = {
    patterns: patterns.map((pattern) => ({ ...pattern })),
    engine,
    scope: scope.trim() || 'HEAD',
    allRefs: searchAllRefs,
    author: author.trim(),
    since: since.trim(),
    until: until.trim(),
  } satisfies SearchDraftSnapshot
  $: searchStale = Boolean(executedSearchDraft && !sameSearchDraft(executedSearchDraft, searchDraft))
  $: activeSearchSessionState = {
    projectRoot: activeProjectRoot,
    worktreeRoot: repository?.root ?? '',
    draft: cloneSearchDraft(searchDraft),
    results: [...results],
    selectedIndex: selectedSearchIndex,
    scanned,
    resultScope,
    resultAllRefs,
    executedDraft: executedSearchDraft ? cloneSearchDraft(executedSearchDraft) : null,
    hasSearched,
    lastSearchedAt: searchLastSearchedAt || undefined,
    error: searchError,
  }
  $: searchSessionSummaries = searchSessions.map((session) => summarizeSearchSession(
    navigatorView === 'search' && session.id === activeSearchSessionID
      ? { ...session, ...activeSearchSessionState }
      : session,
    navigatorView === 'search' && session.id === activeSearchSessionID && searching,
  ))

  // Wails can create the WebView before Svelte's mount callback is resumed
  // when a bundled app is opened through NSWorkspace. Schedule bridge
  // hydration after the first render so Finder and direct launches behave alike.
  setTimeout(() => void initialise(), 0)

  onMount(() => {
    readPaneWidths()
    const stopSearchProgress = api.onSearchProgress((progress) => {
      if (!searching || progress.request_id !== searchRequestID) return
      searchProgress = progress
    })
    const keydown = (event: KeyboardEvent) => {
      if (event.metaKey && event.key === ',') {
        event.preventDefault()
        settingsOpen = true
      }
    }
    window.addEventListener('keydown', keydown)
    return () => {
      stopSearchProgress()
      window.removeEventListener('keydown', keydown)
    }
  })

  async function initialise(): Promise<void> {
    appSettings = readAppSettings()
    if (!api.available()) {
      setStatus('Desktop bridge unavailable. Run GitGit with Wails.', 'error')
      return
    }
    void api.version().then((version) => (appVersion = version)).catch(() => undefined)
    try {
      const state = await api.initialState()
      await loadProjects()
      await activateRepository(state)
    } catch (error) {
      setStatus(errorText(error), 'error')
      await loadProjects()
    }
  }

  async function loadProjects(): Promise<void> {
    try {
      projects = await api.projects()
    } catch (error) {
      projects = []
      setStatus(errorText(error), 'error')
    }
  }

  async function activateRepository(state: RepositoryState, projectRoot = state.project_root || state.root, initialHistoryScope?: string): Promise<void> {
    repository = state
    activeProjectRoot = projectRoot
    historyAllBranches = initialHistoryScope === undefined
    historyScope = initialHistoryScope ?? state.default_branch ?? 'HEAD'
    scope = 'HEAD'
    searchAllRefs = false
    activePresetIDs = []
    historyFilterProgress = null
    clearSearchState()
    setStatus()
    projectSwitching = false
    worktreeSwitching = false
    historyLoading = false
    await loadHistory()
  }

  function commitEditDisabledReason(): string {
    if (!repository || !selectedCommit) return 'Select a commit to edit.'
    if (projectSwitching) return 'Wait for the project switch to finish.'
    if (worktreeSwitching) return 'Wait for the worktree switch to finish.'
    if (currentWorktreeDetached) return 'Commit editing requires a worktree with a local branch checked out.'
    if (historyAllBranches || historyScope !== repository.branch) return `Select the checked-out branch ${repository.branch} before editing commits.`
    return ''
  }

  async function loadHistory(): Promise<void> {
    if (!repository || historyLoading) return
    const requestID = ++historyRequestID
    const repositoryRoot = repository.root
    historyLoading = true
    historyLoadingMore = false
    historyFilterProgress = null
    historyLoadMorePromise = null
    setStatus()
    try {
      await tick()
      const response = await api.history({ scope: historyScope, all_branches: historyAllBranches, related_scope: relatedHistoryScope(), limit: historyBatchSize(), skip: 0 })
      if (requestID !== historyRequestID || repository?.root !== repositoryRoot) return
      history = response
      historyAutoLoadEnabled = !response.branch_point || response.commits.length >= response.total
      historyLoading = false
      const selected = history.commits.find((commit) => commit.commit === selectedCommit) ?? history.commits[0]
      if (selected) void selectCommit(selected)
      else {
        selectedCommit = ''
        historyDetail = null
      }
      if (filterRules.length > 0) void applyCommitFilters(filterRules, appSettings.filter_logic)
    } catch (error) {
      if (requestID !== historyRequestID || repository?.root !== repositoryRoot) return
      setStatus(errorText(error), 'error')
      history = { scope: historyAllBranches ? 'All branches' : historyScope, all_branches: historyAllBranches, total: 0, branches: [], commits: [] }
      historyAutoLoadEnabled = true
      selectedCommit = ''
      historyDetail = null
    } finally {
      if (requestID === historyRequestID) historyLoading = false
    }
  }

  function loadMoreHistory(): Promise<boolean> {
    if (historyLoadMorePromise) return historyLoadMorePromise
    if (!repository || historyLoading || history.commits.length >= history.total) return Promise.resolve(false)
    const task = fetchMoreHistory()
    historyLoadMorePromise = task
    void task.then(() => {
      if (historyLoadMorePromise === task) historyLoadMorePromise = null
    })
    return task
  }

  async function fetchMoreHistory(): Promise<boolean> {
    if (!repository) return false
    const requestID = historyRequestID
    historyLoadingMore = true
    const repositoryRoot = repository.root
    const requestedScope = historyScope
    const requestedAllBranches = historyAllBranches
    const requestedRelatedScope = relatedHistoryScope()
    try {
      const response = await api.history({
        scope: requestedScope,
        all_branches: requestedAllBranches,
        related_scope: requestedRelatedScope,
        limit: historyBatchSize(),
        skip: history.commits.length,
      })
      if (requestID !== historyRequestID || repository?.root !== repositoryRoot || historyScope !== requestedScope || historyAllBranches !== requestedAllBranches || relatedHistoryScope() !== requestedRelatedScope) return false
      const known = new Set(history.commits.map((commit) => commit.commit))
      const appended = response.commits.filter((commit) => !known.has(commit.commit))
      history = {
        ...response,
        commits: [...history.commits, ...appended],
      }
      historyAutoLoadEnabled = true
      return appended.length > 0
    } catch (error) {
      if (requestID === historyRequestID) setStatus(errorText(error), 'error')
      return false
    } finally {
      if (requestID === historyRequestID) historyLoadingMore = false
    }
  }

  async function loadMoreHistoryForCurrentFilters(): Promise<void> {
    const loaded = await loadMoreHistory()
    if (!loaded) return
    if (filterRules.some((rule) => rule.field === 'branch')) await loadHistoryBranches(filterRules)
    reconcileVisibleSelection(filterRules, appSettings.filter_logic)
  }

  function historyBatchSize(): number {
    if (appSettings.history_batch_size > 0) return appSettings.history_batch_size
    const tableHeight = document.querySelector<HTMLElement>('.commit-table-scroll')?.clientHeight
      ?? Math.max(400, window.innerHeight - 160)
    return clamp(Math.ceil(tableHeight / commitRowHeight) * 2, 24, 100)
  }

  function relatedHistoryScope(): string {
    if (!repository || historyAllBranches || historyScope === repository.default_branch) return ''
    return repository.default_branch
  }

  async function changeHistoryScope(nextScope: string, nextAllBranches: boolean): Promise<void> {
    historyScope = nextScope
    historyAllBranches = nextAllBranches
    selectedCommit = ''
    historyDetail = null
    detailOverride = null
    await loadHistory()
  }

  async function openCommitEditor(): Promise<void> {
    if (!canEditCommits || !repository || !selectedCommit || commitEditorLoading || commitEditorApplying) {
      if (editDisabledReason) setStatus(editDisabledReason, 'warning')
      return
    }
    const requestID = ++commitEditorRequestID
    const repositoryRoot = repository.root
    commitEditorOpen = true
    commitEditorStack = null
    commitEditorLoading = true
    commitEditorError = ''
    try {
      const stack = await api.prepareCommitEdit(selectedCommit)
      if (requestID !== commitEditorRequestID || repository?.root !== repositoryRoot) return
      commitEditorStack = stack
    } catch (error) {
      if (requestID !== commitEditorRequestID || repository?.root !== repositoryRoot) return
      commitEditorError = errorText(error)
    } finally {
      if (requestID === commitEditorRequestID) commitEditorLoading = false
    }
  }

  function closeCommitEditor(): void {
    if (commitEditorApplying) return
    commitEditorRequestID++
    commitEditorOpen = false
    commitEditorStack = null
    commitEditorLoading = false
    commitEditorError = ''
  }

  async function loadCommitFileContent(commit: string, path: string): Promise<CommitFileContent> {
    return api.commitFileContent(commit, path)
  }

  async function applyCommitRewrite(request: RewriteCommitsRequest): Promise<void> {
    if (!repository || commitEditorApplying) return
    const repositoryRoot = repository.root
    commitEditorApplying = true
    commitEditorError = ''
    try {
      const result = await api.rewriteCommits(request)
      if (repository?.root !== repositoryRoot) return
      repository = result.state
      commitEditorOpen = false
      commitEditorStack = null
      selectedCommit = result.head
      historyDetail = null
      detailOverride = null
      await loadHistory()
      if (repository?.root !== repositoryRoot) return
      const rewriteStatus = `Commit history rewritten · backup ${result.backup_ref}`
      if (result.warning) setStatus(`${rewriteStatus} · ${result.warning}`, 'warning')
      else if (!statusMessage) setStatus(rewriteStatus, 'success')
    } catch (error) {
      if (repository?.root === repositoryRoot) commitEditorError = errorText(error)
    } finally {
      commitEditorApplying = false
    }
  }

  async function selectCommit(commit: CommitSummary): Promise<void> {
    if (selectedCommit === commit.commit && historyDetail) return
    const requestID = ++detailRequestID
    const repositoryRoot = repository?.root
    const requestedAllBranches = historyAllBranches
    selectedCommit = commit.commit
    detailOverride = null
    try {
      const [detail, branchResponse] = await Promise.all([
        api.commitDetail(commit.commit, ''),
        requestedAllBranches && commit.branches === undefined
          ? api.historyBranches([commit.commit])
          : Promise.resolve(null),
      ])
      if (requestID !== detailRequestID || repository?.root !== repositoryRoot || selectedCommit !== commit.commit || historyAllBranches !== requestedAllBranches) return
      historyDetail = {
        ...detail,
        historical_branch: commit.historical_branch,
        branches: branchResponse?.branches[commit.commit] ?? commit.branches,
      }
    } catch (error) {
      if (requestID !== detailRequestID || repository?.root !== repositoryRoot) return
      setStatus(errorText(error), 'error')
      historyDetail = null
    }
  }

  async function loadHistoryBranches(rules = filterRules): Promise<void> {
    if (!repository || branchMembershipLoading || !rules.some((rule) => rule.field === 'branch')) return
    const commits = history.commits.filter((commit) => commit.branches === undefined)
    if (commits.length === 0) return
    const requestID = ++branchRequestID
    const repositoryRoot = repository.root
    branchMembershipLoading = true
    try {
      const response = await api.historyBranches(commits.map((commit) => commit.commit))
      if (requestID !== branchRequestID || repository?.root !== repositoryRoot) return
      history = {
        ...history,
        commits: history.commits.map((commit) => ({
          ...commit,
          branches: response.branches[commit.commit] ?? commit.branches ?? [],
        })),
      }
      reconcileVisibleSelection(filterRules, appSettings.filter_logic)
    } catch (error) {
      if (requestID !== branchRequestID || repository?.root !== repositoryRoot) return
      const message = errorText(error)
      if (!isCancellationMessage(message)) setStatus(message, 'error')
    } finally {
      if (requestID === branchRequestID) branchMembershipLoading = false
    }
  }

  async function applyCommitFilters(rules: CommitFilterRule[], logic: CommitFilterLogic): Promise<void> {
    const requestID = ++filterRequestID
    if (rules.length === 0) {
      historyFilterProgress = null
      reconcileVisibleSelection(rules, logic)
      return
    }
    const target = historyBatchSize()
    if (visibleCommits(history.commits, rules, logic).length < target && history.commits.length < history.total) {
      historyFilterProgress = commitFilterProgress(rules, target, logic)
    }
    if (rules.some((rule) => rule.field === 'branch')) await loadHistoryBranches(rules)
    while (
      requestID === filterRequestID
      && visibleCommits(history.commits, rules, logic).length < target
      && history.commits.length < history.total
    ) {
      historyFilterProgress = commitFilterProgress(rules, target, logic)
      const loaded = await loadMoreHistory()
      if (!loaded || requestID !== filterRequestID) break
      if (rules.some((rule) => rule.field === 'branch')) await loadHistoryBranches(rules)
    }
    if (requestID === filterRequestID) {
      historyFilterProgress = null
      reconcileVisibleSelection(rules, logic)
    }
  }

  function commitFilterProgress(rules: CommitFilterRule[], target: number, logic: CommitFilterLogic): HistoryFilterProgress {
    const presets = appSettings.presets
      .filter((preset) => activePresetIDs.includes(preset.id))
      .map((preset) => preset.label)
    return {
      presets,
      conditions: rules.map((rule) => `${actionLabel(rule.action)} ${rule.field}: ${rule.pattern}`),
      scope: historyAllBranches ? 'All branches' : historyScope,
      target,
      visible: visibleCommits(history.commits, rules, logic).length,
      scanned: history.commits.length,
      total: history.total,
    }
  }

  function reconcileVisibleSelection(rules: CommitFilterRule[], logic: CommitFilterLogic): void {
    const visible = visibleCommits(history.commits, rules, logic)
    if (visible.length > 0 && !visible.some((commit) => commit.commit === selectedCommit)) {
      void selectCommit(visible[0])
    } else if (visible.length === 0) {
      selectedCommit = ''
      historyDetail = null
      detailOverride = null
    }
  }

  function togglePreset(id: string): void {
    activePresetIDs = activePresetIDs.includes(id)
      ? activePresetIDs.filter((activeID) => activeID !== id)
      : [...activePresetIDs, id]
    void applyCommitFilters(resolvePresetRules(appSettings.presets, activePresetIDs, repository?.user ?? { name: '', email: '' }), appSettings.filter_logic)
  }

  function actionLabel(action: CommitFilterAction): string {
    return `${action[0].toLocaleUpperCase()}${action.slice(1)}`
  }

  async function chooseRepository(): Promise<void> {
    if (repositoryTransitioning) return
    if (!api.available()) {
      setStatus('Desktop bridge unavailable. Run GitGit with Wails.', 'error')
      return
    }
    const requestID = beginRepositoryTransition()
    try {
      const state = await api.chooseRepository()
      await loadProjects()
      if (requestID !== repositoryRequestID) return
      await activateRepository(state)
    } catch (error) {
      if (requestID !== repositoryRequestID) return
      setStatus(errorText(error), 'error')
      projectSwitching = false
      historyLoading = false
    }
  }

  async function selectProject(project: RegisteredProject): Promise<void> {
    if (repositoryTransitioning || (project.root === activeProjectRoot && repository)) return
    const requestID = beginRepositoryTransition()
    try {
      const state = await api.openRepository(project.root)
      if (requestID !== repositoryRequestID) return
      await activateRepository(state, project.root)
    } catch (error) {
      if (requestID !== repositoryRequestID) return
      setStatus(errorText(error), 'error')
      projectSwitching = false
      historyLoading = false
    }
  }

  function beginRepositoryTransition(): number {
    const requestID = ++repositoryRequestID
    historyRequestID++
    detailRequestID++
    branchRequestID++
    searchRequestID++
    syncRequestID++
    if (searching) void api.cancelSearch()
    historyLoading = false
    historyLoadingMore = false
    historyFilterProgress = null
    branchMembershipLoading = false
    searching = false
    syncing = false
    refreshing = false
    projectSwitching = true
    commitEditorRequestID++
    commitEditorOpen = false
    commitEditorStack = null
    commitEditorLoading = false
    commitEditorApplying = false
    commitEditorError = ''
    setStatus('Switching project…')
    return requestID
  }

  function beginWorktreeTransition(): number {
    const requestID = ++repositoryRequestID
    historyRequestID++
    detailRequestID++
    branchRequestID++
    searchRequestID++
    syncRequestID++
    commitEditorRequestID++
    if (searching) void api.cancelSearch()
    historyLoading = false
    historyLoadingMore = false
    branchMembershipLoading = false
    searching = false
    syncing = false
    refreshing = false
    worktreeSwitching = true
    commitEditorOpen = false
    commitEditorStack = null
    commitEditorLoading = false
    commitEditorApplying = false
    commitEditorError = ''
    setStatus('Switching worktree…')
    return requestID
  }

  async function toggleProjectFavorite(project: RegisteredProject): Promise<void> {
    try {
      projects = await api.setProjectFavorite(project.root, !project.favorite)
      setStatus()
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function discoverProjects(): Promise<void> {
    if (discoveringProjects) return
    discoveringProjects = true
    discoveryMessage = ''
    try {
      const result = await api.discoverProjects()
      projects = result.projects
      if (!result.canceled) {
        discoveryMessage = `Found ${result.found} Git project${result.found === 1 ? '' : 's'} · registered ${result.added} new.`
      }
    } catch (error) {
      discoveryMessage = errorText(error)
    } finally {
      discoveringProjects = false
    }
  }

  function updateHistoryBatchSize(value: number): void {
    const normalized = [0, 50, 100, 200, 500].includes(value) ? value : 0
    appSettings = { ...appSettings, history_batch_size: normalized }
    saveAppSettings()
  }

  function updateIDE(ide: IDEPreference): void {
    appSettings = { ...appSettings, ide }
    saveAppSettings()
  }

  function updateTerminal(terminal: TerminalPreference): void {
    appSettings = { ...appSettings, terminal }
    saveAppSettings()
  }

  function updateChangedFilesView(changedFilesView: ChangedFilesView): void {
    appSettings = { ...appSettings, changed_files_view: changedFilesView }
    saveAppSettings()
  }

  function updatePresets(presets: CommitFilterPreset[]): void {
    const nextPresets = cloneFilterPresets(presets)
    appSettings = { ...appSettings, presets: nextPresets }
    activePresetIDs = activePresetIDs.filter((id) => nextPresets.some((preset) => preset.id === id))
    saveAppSettings()
    void applyCommitFilters(resolvePresetRules(nextPresets, activePresetIDs, repository?.user ?? { name: '', email: '' }), appSettings.filter_logic)
  }

  function resetPresets(): void {
    updatePresets(defaultFilterPresets())
  }

  function updateRemoteBadges(remoteBadges: RemoteBadgeRule[]): void {
    appSettings = {
      ...appSettings,
      remote_badges: remoteBadges.map((rule) => ({ ...rule, icon: normalizeRemoteBadgeIcon(rule.icon) })),
    }
    saveAppSettings()
  }

  function saveAppSettings(): void {
    localStorage.setItem(settingsKey, JSON.stringify(appSettings))
  }

  function defaultAppSettings(): AppSettings {
    return {
      history_batch_size: 0,
      ide: 'vscode',
      terminal: 'terminal',
      changed_files_view: 'list',
      filter_logic: { ...defaultFilterLogic },
      presets: defaultFilterPresets(),
      remote_badges: defaultRemoteBadgeRules(),
    }
  }

  function readAppSettings(): AppSettings {
    try {
      const parsed = JSON.parse(localStorage.getItem(settingsKey) ?? '{}')
      const value = Number(parsed.history_batch_size)
      const ide = ['vscode', 'cursor', 'zed', 'idea', 'xcode'].includes(parsed.ide) ? parsed.ide as IDEPreference : 'vscode'
      const terminal = ['terminal', 'iterm2', 'warp', 'ghostty', 'wezterm'].includes(parsed.terminal) ? parsed.terminal as TerminalPreference : 'terminal'
      const changedFilesView = parsed.changed_files_view === 'tree' ? 'tree' : 'list'
      return {
        history_batch_size: [0, 50, 100, 200, 500].includes(value) ? value : 0,
        ide,
        terminal,
        changed_files_view: changedFilesView,
        filter_logic: { ...defaultFilterLogic },
        presets: readFilterPresets(parsed.presets),
        remote_badges: readRemoteBadgeRules(parsed.remote_badges),
      }
    } catch {
      return defaultAppSettings()
    }
  }

  function readFilterPresets(value: unknown): CommitFilterPreset[] {
    if (value === undefined) return defaultFilterPresets()
    if (!Array.isArray(value)) return defaultFilterPresets()
    const actions: CommitFilterAction[] = ['hide', 'show']
    const fields: CommitFilterField[] = ['branch', 'author', 'message', 'file', 'date']
    return value.flatMap((candidate, presetIndex) => {
      if (!candidate || typeof candidate !== 'object') return []
      const preset = candidate as Partial<CommitFilterPreset>
      if (!Array.isArray(preset.rules)) return []
      const id = typeof preset.id === 'string' && preset.id.trim() ? preset.id : `preset-${presetIndex}`
      const label = typeof preset.label === 'string' && preset.label.trim() ? preset.label : `Preset ${presetIndex + 1}`
      const rules = preset.rules.flatMap((candidateRule, ruleIndex) => {
        if (!candidateRule || typeof candidateRule !== 'object') return []
        const rule = candidateRule as Partial<CommitFilterRule>
        if (!actions.includes(rule.action as CommitFilterAction) || !fields.includes(rule.field as CommitFilterField) || typeof rule.pattern !== 'string') return []
        return [{
          id: typeof rule.id === 'string' && rule.id ? rule.id : `${id}-rule-${ruleIndex}`,
          action: rule.action as CommitFilterAction,
          field: rule.field as CommitFilterField,
          pattern: rule.pattern,
        }]
      })
      return rules.length > 0 ? [{ id, label, rules }] : []
    })
  }

  function readRemoteBadgeRules(value: unknown): RemoteBadgeRule[] {
    if (value === undefined) return defaultRemoteBadgeRules()
    if (!Array.isArray(value)) return defaultRemoteBadgeRules()
    return value.flatMap((candidate, index) => {
      if (!candidate || typeof candidate !== 'object') return []
      const rule = candidate as Partial<RemoteBadgeRule>
      if (typeof rule.pattern !== 'string' || typeof rule.icon !== 'string') return []
      return [{
        id: typeof rule.id === 'string' && rule.id ? rule.id : `remote-badge-${index}`,
        pattern: rule.pattern,
        icon: normalizeRemoteBadgeIcon(rule.icon),
      }]
    })
  }

  async function refreshRepository(): Promise<void> {
    if (!repository || refreshing) return
    const repositoryRoot = repository.root
    refreshing = true
    try {
      const state = await api.refresh()
      if (repository?.root !== repositoryRoot) return
      repository = state
      await loadHistory()
    } catch (error) {
      if (repository?.root === repositoryRoot) setStatus(errorText(error), 'error')
    } finally {
      if (repository?.root === repositoryRoot) refreshing = false
    }
  }

  async function syncRemotes(): Promise<void> {
    if (!repository || syncing) return
    const requestID = ++syncRequestID
    const repositoryRoot = repository.root
    syncing = true
    setStatus()
    try {
      const result = await api.syncRemotes()
      if (requestID !== syncRequestID || repository?.root !== repositoryRoot) return
      repository = result.state
      if (result.warnings?.length) setStatus(result.warnings.join(' · '), 'warning')
      else setStatus('Remote sync complete.', 'success')
      await loadHistory()
    } catch (error) {
      if (requestID !== syncRequestID || repository?.root !== repositoryRoot) return
      const message = errorText(error)
      if (!isCancellationMessage(message)) setStatus(message, 'error')
    } finally {
      if (requestID === syncRequestID) syncing = false
    }
  }

  async function selectWorktree(worktree: WorktreeInfo): Promise<void> {
    if (repositoryTransitioning || worktree.path === repository?.root) return
    const projectRoot = repository?.project_root || activeProjectRoot
    const requestID = beginWorktreeTransition()
    try {
      const state = await api.openRepository(worktree.path)
      if (requestID !== repositoryRequestID) return
      await activateRepository(state, projectRoot, worktreeHistoryScope(state.branch, worktree.detached))
    } catch (error) {
      if (requestID !== repositoryRequestID) return
      setStatus(errorText(error), 'error')
      worktreeSwitching = false
    }
  }

  async function openWorktree(worktree: WorktreeInfo): Promise<void> {
    try {
      await api.openWorktree(worktree.path)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function openWorktreeInIDE(worktree: WorktreeInfo): Promise<void> {
    try {
      await api.openWorktreeInIDE(worktree.path, appSettings.ide)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function removeMergedWorktrees(worktrees: WorktreeInfo[]): Promise<boolean> {
    if (!repository || removingWorktrees || worktrees.length === 0) return false
    const repositoryRoot = repository.root
    const requestID = repositoryRequestID
    removingWorktrees = true
    try {
      const state = await api.removeMergedWorktrees(worktrees.map((worktree) => worktree.path))
      if (requestID !== repositoryRequestID || repository?.root !== repositoryRoot) return false
      await activateRepository(state, state.project_root || activeProjectRoot)
      return true
    } catch (error) {
      if (requestID !== repositoryRequestID || repository?.root !== repositoryRoot) return false
      setStatus(errorText(error), 'error')
      try {
        const refreshed = await api.refresh()
        if (requestID === repositoryRequestID && repository?.root === repositoryRoot) repository = refreshed
      } catch {
        // Preserve the actionable removal error when refresh also fails.
      }
      return false
    } finally {
      removingWorktrees = false
    }
  }

  async function addPatternSearch(source: Pattern['source'], value: string): Promise<void> {
    const normalized = value.trim()
    if (!normalized) return
    const searchValue = source === 'msg' ? `*${escapeGlob(normalized)}*` : normalized
    createSearchSession([{ source, value: searchValue }])
    setStatus(`${source === 'msg' ? 'Message' : source.toUpperCase()} condition added to a new Search session.`, 'info')
  }

  function changeSearchScope(nextScope: string, nextAllRefs: boolean): void {
    scope = nextAllRefs ? 'All refs' : nextScope
    searchAllRefs = nextAllRefs
  }

  function escapeGlob(value: string): string {
    return value.replaceAll('\\', '\\\\').replaceAll('*', '\\*').replaceAll('?', '\\?').replaceAll('[', '\\[')
  }

  function blankSearchDraft(initialPatterns: Pattern[] = [], initialAuthor = ''): SearchDraftSnapshot {
    return {
      patterns: initialPatterns.map((pattern, index) => ({
        ...pattern,
        join: index === 0 ? undefined : (pattern.join ?? 'and'),
      })),
      engine: 'glob',
      scope: historyAllBranches ? 'All refs' : historyScope || 'HEAD',
      allRefs: historyAllBranches,
      author: initialAuthor.trim(),
      since: '',
      until: '',
    }
  }

  function createSearchSession(initialPatterns: Pattern[] = [], initialAuthor = ''): void {
    if (navigatorView === 'search') storeActiveSearchSession()
    const id = `search-${++searchSessionSequence}`
    const session: SearchSessionState = {
      id,
      title: `Search ${searchSessionSequence}`,
      projectRoot: activeProjectRoot,
      worktreeRoot: repository?.root ?? '',
      draft: blankSearchDraft(initialPatterns, initialAuthor),
      results: [],
      selectedIndex: -1,
      scanned: 0,
      resultScope: 'HEAD',
      resultAllRefs: false,
      executedDraft: null,
      hasSearched: false,
      lastSearchedAt: undefined,
      error: '',
    }
    searchSessions = [...searchSessions, session]
    activeSearchSessionID = id
    navigatorView = 'search'
    restoreSearchSession(session)
    void tick().then(focusPatternInput)
  }

  async function changeNavigatorView(nextView: NavigatorView): Promise<void> {
    if (navigatorView === 'search') storeActiveSearchSession()
    if (nextView !== 'search') {
      navigatorView = nextView
      return
    }
    if (searchSessions.length === 0) {
      navigatorView = 'search'
      activeSearchSessionID = ''
      resetSearchWorkspace()
      return
    }
    await selectSearchSession(activeSearchSessionID || searchSessions[0].id, true)
  }

  async function selectSearchSession(id: string, force = false): Promise<void> {
    if (!force && id === activeSearchSessionID) return
    const previousSessionID = activeSearchSessionID
    if (navigatorView === 'search' && activeSearchSessionID) storeActiveSearchSession()
    const session = searchSessions.find((candidate) => candidate.id === id)
    if (!session) return
    if (searching) await cancelSearch()
    activeSearchSessionID = id
    navigatorView = 'search'
    if (session.worktreeRoot && repository?.root !== session.worktreeRoot) {
      const requestID = beginRepositoryTransition()
      try {
        const state = await api.openRepository(session.worktreeRoot)
        if (requestID !== repositoryRequestID) return
        await activateRepository(state, session.projectRoot || state.project_root || state.root)
      } catch (error) {
        if (requestID === repositoryRequestID) {
          projectSwitching = false
          setStatus(errorText(error), 'error')
        }
        activeSearchSessionID = previousSessionID
        const previousSession = searchSessions.find((candidate) => candidate.id === previousSessionID)
        if (previousSession) restoreSearchSession(previousSession)
        return
      }
    }
    restoreSearchSession(session)
  }

  function storeActiveSearchSession(): void {
    if (!activeSearchSessionID) return
    searchSessions = searchSessions.map((session) => session.id === activeSearchSessionID
      ? captureActiveSearchSession(session)
      : session)
  }

  function captureActiveSearchSession(session: SearchSessionState): SearchSessionState {
    return {
      ...session,
      projectRoot: activeProjectRoot || session.projectRoot,
      worktreeRoot: repository?.root ?? session.worktreeRoot,
      draft: cloneSearchDraft(searchDraft),
      results: [...results],
      selectedIndex: selectedSearchIndex,
      scanned,
      resultScope,
      resultAllRefs,
      executedDraft: executedSearchDraft ? cloneSearchDraft(executedSearchDraft) : null,
      hasSearched,
      lastSearchedAt: searchLastSearchedAt || session.lastSearchedAt,
      error: searchError,
    }
  }

  function restoreSearchSession(session: SearchSessionState): void {
    const draft = cloneSearchDraft(session.draft)
    patterns = draft.patterns
    engine = draft.engine
    scope = draft.scope
    searchAllRefs = draft.allRefs
    author = draft.author
    since = draft.since
    until = draft.until
    results = [...session.results]
    selectedSearchIndex = session.selectedIndex
    scanned = session.scanned
    resultScope = session.resultScope
    resultAllRefs = session.resultAllRefs
    executedSearchDraft = session.executedDraft ? cloneSearchDraft(session.executedDraft) : null
    hasSearched = session.hasSearched
    searchLastSearchedAt = session.lastSearchedAt ?? ''
    searchError = session.error
  }

  function resetSearchWorkspace(): void {
    const draft = blankSearchDraft()
    patterns = draft.patterns
    engine = draft.engine
    scope = draft.scope
    searchAllRefs = draft.allRefs
    author = draft.author
    since = draft.since
    until = draft.until
    results = []
    selectedSearchIndex = -1
    scanned = 0
    resultScope = 'HEAD'
    resultAllRefs = false
    executedSearchDraft = null
    hasSearched = false
    searchLastSearchedAt = ''
    searchError = ''
  }

  function renameSearchSession(id: string, title: string): void {
    searchSessions = searchSessions.map((session) => session.id === id ? { ...session, title: title.slice(0, 80) } : session)
  }

  async function removeSearchSession(id: string): Promise<void> {
    const index = searchSessions.findIndex((session) => session.id === id)
    if (index < 0) return
    if (id === activeSearchSessionID && searching) await cancelSearch()
    const remaining = searchSessions.filter((session) => session.id !== id)
    const next = remaining[Math.min(index, remaining.length - 1)]
    searchSessions = remaining
    if (id !== activeSearchSessionID) return
    activeSearchSessionID = ''
    if (next) {
      await selectSearchSession(next.id, true)
    } else {
      resetSearchWorkspace()
    }
  }

  function summarizeSearchSession(session: SearchSessionState, running = false) {
    const project = projects.find((candidate) => candidate.root === session.projectRoot)?.name
      ?? session.projectRoot.split('/').filter(Boolean).at(-1)
      ?? ''
    const status: SearchSessionStatus | undefined = running
      ? 'running'
      : session.error
        ? 'error'
        : session.executedDraft && !sameSearchDraft(session.executedDraft, session.draft)
          ? 'stale'
          : session.executedDraft
            ? 'ready'
            : undefined
    return {
      id: session.id,
      title: session.title,
      project,
      query: session.draft.patterns.map(searchPatternText).join(' '),
      status,
      result_count: searchResultCommitCount(session.results),
      last_searched_at: session.lastSearchedAt,
    }
  }

  async function selectSearchProject(project: RegisteredProject): Promise<void> {
    await retargetSearchSession(project.root, project.root, false)
  }

  async function chooseSearchProject(): Promise<void> {
    if (!activeSearchSessionID || repositoryTransitioning) return
    const current = searchSessions.find((session) => session.id === activeSearchSessionID)
    if (!current) return
    const preserved = captureActiveSearchSession(current)
    const requestID = beginRepositoryTransition()
    try {
      const state = await api.chooseRepository()
      if (requestID !== repositoryRequestID) return
      await loadProjects()
      await activateRepository(state, state.project_root || state.root, state.branch || 'HEAD')
      const retargeted: SearchSessionState = {
        ...preserved,
        projectRoot: state.project_root || state.root,
        worktreeRoot: state.root,
        draft: { ...preserved.draft, scope: state.branch || 'HEAD', allRefs: false },
        results: [], selectedIndex: -1, scanned: 0,
        resultScope: 'HEAD', resultAllRefs: false,
        executedDraft: null, hasSearched: false, lastSearchedAt: undefined, error: '',
      }
      searchSessions = searchSessions.map((session) => session.id === retargeted.id ? retargeted : session)
      restoreSearchSession(retargeted)
      navigatorView = 'search'
    } catch (error) {
      if (requestID === repositoryRequestID) {
        projectSwitching = false
        setStatus(errorText(error), 'error')
      }
    }
  }

  async function selectSearchWorktree(worktree: WorktreeInfo): Promise<void> {
    await retargetSearchSession(worktree.path, activeProjectRoot, true)
  }

  async function retargetSearchSession(path: string, projectRoot: string, worktreeChange: boolean): Promise<void> {
    if (!activeSearchSessionID || repositoryTransitioning || path === repository?.root) return
    const current = searchSessions.find((session) => session.id === activeSearchSessionID)
    if (!current) return
    const preserved = captureActiveSearchSession(current)
    const requestID = worktreeChange ? beginWorktreeTransition() : beginRepositoryTransition()
    try {
      const state = await api.openRepository(path)
      if (requestID !== repositoryRequestID) return
      await activateRepository(state, projectRoot || state.project_root || state.root, state.branch || 'HEAD')
      const retargeted: SearchSessionState = {
        ...preserved,
        projectRoot: projectRoot || state.project_root || state.root,
        worktreeRoot: state.root,
        draft: { ...preserved.draft, scope: state.branch || 'HEAD', allRefs: false },
        results: [],
        selectedIndex: -1,
        scanned: 0,
        resultScope: 'HEAD',
        resultAllRefs: false,
        executedDraft: null,
        hasSearched: false,
        lastSearchedAt: undefined,
        error: '',
      }
      searchSessions = searchSessions.map((session) => session.id === retargeted.id ? retargeted : session)
      restoreSearchSession(retargeted)
      navigatorView = 'search'
    } catch (error) {
      if (requestID === repositoryRequestID) {
        projectSwitching = false
        worktreeSwitching = false
        setStatus(errorText(error), 'error')
      }
    }
  }

  function searchRequest(requestID: number): SearchRequest {
    return {
      request_id: requestID,
      patterns: patterns.map((pattern) => ({ ...pattern })),
      engine,
      scope: scope.trim() || 'HEAD',
      all_refs: searchAllRefs,
      author: author.trim(),
      since: normalizeSearchBoundary(since, 'since'),
      until: normalizeSearchBoundary(until, 'until'),
      follow_rename: patterns.some((pattern) => pattern.source === 'file'),
      limit: 250,
      context: 3,
    }
  }

  async function runSearch(): Promise<void> {
    if (!repository || searching) return
    if (patterns.length === 0) {
      setStatus('Add at least one Message, DIFF, or FILE pattern.', 'warning')
      await tick()
      focusPatternInput()
      return
    }
    if (patterns.some((pattern) => !pattern.value.trim())) {
      setStatus('Complete or remove empty Search conditions.', 'warning')
      return
    }
    const expressionError = searchExpressionError(patterns)
    if (expressionError) {
      setStatus(expressionError, 'warning')
      return
    }
    const draft = cloneSearchDraft(searchDraft)
    const requestID = ++searchRequestID
    const request = searchRequest(requestID)
    searchLastSearchedAt = new Date().toISOString()
    searching = true
    searchProgress = { request_id: requestID, scanned: 0, total: 0 }
    searchError = ''
    setStatus()
    const repositoryRoot = repository.root
    try {
      const response = await api.search(request)
      if (requestID !== searchRequestID || repository?.root !== repositoryRoot) return
      results = response.results
      scanned = response.scanned
      resultScope = response.scope
      resultAllRefs = response.all_refs
      executedSearchDraft = draft
      hasSearched = true
      selectedSearchIndex = searchResultCommitCount(results) > 0 ? 0 : -1
    } catch (error) {
      if (requestID !== searchRequestID || repository?.root !== repositoryRoot) return
      const message = errorText(error)
      if (!isCancellationMessage(message)) {
        searchError = message
        setStatus(message, 'error')
      }
    } finally {
      if (requestID === searchRequestID) {
        searching = false
        searchProgress = null
        storeActiveSearchSession()
      }
    }
  }

  async function cancelSearch(): Promise<void> {
    if (!searching) return
    searchRequestID++
    searchProgress = null
    try {
      await api.cancelSearch()
    } finally {
      searching = false
      storeActiveSearchSession()
    }
  }

  function clearSearchState(): void {
    patterns = []
    results = []
    selectedSearchIndex = -1
    scanned = 0
    hasSearched = false
    resultScope = 'HEAD'
    resultAllRefs = false
    executedSearchDraft = null
    searchError = ''
    searchProgress = null
    detailOverride = null
  }

  function readPaneWidths(): void {
    try {
      const value = JSON.parse(localStorage.getItem(paneWidthKey) ?? '{}')
      if (Number.isFinite(value.inspector)) inspectorWidth = clamp(value.inspector, 360, inspectorMaximumWidth())
    } catch {
      // Keep defaults when persisted layout state is invalid.
    }
  }

  function savePaneWidths(): void {
    localStorage.setItem(paneWidthKey, JSON.stringify({ inspector: inspectorWidth }))
  }

  function startPaneResize(event: MouseEvent): void {
    event.preventDefault()
    const startX = event.clientX
    const startWidth = inspectorWidth
    const workspaceWidth = document.querySelector<HTMLElement>('.workspace')?.clientWidth ?? window.innerWidth
    document.body.classList.add('resizing-panes')
    const move = (moveEvent: MouseEvent) => {
      const delta = moveEvent.clientX - startX
      const maximum = inspectorMaximumWidth(workspaceWidth)
      inspectorWidth = clamp(startWidth - delta, 360, Math.max(360, maximum))
    }
    const stop = () => {
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', stop)
      document.body.classList.remove('resizing-panes')
      savePaneWidths()
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', stop)
  }

  function resizePaneWithKeyboard(event: KeyboardEvent): void {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    const direction = event.key === 'ArrowRight' ? 1 : -1
    inspectorWidth = clamp(inspectorWidth - direction * 12, 360, inspectorMaximumWidth())
    savePaneWidths()
  }

  function inspectorMaximumWidth(workspaceWidth = document.querySelector<HTMLElement>('.workspace')?.clientWidth ?? window.innerWidth): number {
    return Math.max(360, Math.floor(workspaceWidth * 0.5))
  }

  function clamp(value: number, minimum: number, maximum: number): number {
    return Math.min(Math.max(value, minimum), maximum)
  }

  function focusPatternInput(): void {
    document.querySelector<HTMLInputElement>('[data-pattern-input]')?.focus()
  }

  async function selectInspectorFile(path: string): Promise<CommitDetail | undefined> {
    if (!selectedForInspector || !path) return
    try {
      const detail = await api.commitDetail(selectedForInspector.commit, path)
      detailOverride = detail
      return detail
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  function selectSearchResult(index: number): void {
    if (selectedSearchIndex !== index) detailRequestID++
    selectedSearchIndex = index
  }

  async function selectSearchInspectorFile(path: string): Promise<CommitDetail | undefined> {
    const selected = selectedSearchResult
    if (!selected || !path) return
    const commit = selected.commit
    const requestID = ++detailRequestID
    try {
      const detail = await api.commitDetail(commit, path)
      if (requestID !== detailRequestID || selectedSearchResult?.commit !== commit) return
      return detail
    } catch (error) {
      if (requestID !== detailRequestID || selectedSearchResult?.commit !== commit) return
      setStatus(errorText(error), 'error')
    }
  }

  async function loadRepositoryTree(revision: string, directory: string): Promise<RepositoryTreeResponse> {
    return api.repositoryTree(revision, directory)
  }

  async function copySHA(sha: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(sha)
      setStatus()
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function openFileInIDE(path: string): Promise<void> {
    try {
      await api.openFileInIDE(path, appSettings.ide)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function revealFile(path: string): Promise<void> {
    try {
      await api.revealFile(path)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function openInTerminal(path: string): Promise<void> {
    try {
      await api.openInTerminal(path, appSettings.terminal)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  async function openExternalURL(url: string): Promise<void> {
    try {
      await api.openExternalURL(url)
    } catch (error) {
      setStatus(errorText(error), 'error')
    }
  }

  function cloneSearchDraft(value: SearchDraftSnapshot): SearchDraftSnapshot {
    return { ...value, patterns: value.patterns.map((pattern) => ({ ...pattern })) }
  }

  function sameSearchDraft(left: SearchDraftSnapshot, right: SearchDraftSnapshot): boolean {
    return JSON.stringify(left) === JSON.stringify(right)
  }

  function setStatus(message = '', kind: StatusKind = 'info'): void {
    statusMessage = message
    statusKind = kind
  }

  function errorText(error: unknown): string {
    if (error instanceof Error) return error.message
    if (typeof error === 'string') return error
    return 'An unexpected desktop error occurred.'
  }

  function isCancellationMessage(message: string): boolean {
    const normalized = message.toLocaleLowerCase()
    return normalized.includes('canceled') || normalized.includes('cancelled') || normalized.includes('signal: killed')
  }
</script>

<main class="app-shell">
  <Topbar
    {repository}
    {projects}
    {activeProjectRoot}
    {refreshing}
    {syncing}
    transitioning={repositoryTransitioning}
    view={navigatorView}
    onRegisterProject={() => void chooseRepository()}
    onSelectProject={(project) => void (navigatorView === 'search' ? selectSearchProject(project) : selectProject(project))}
    onToggleProjectFavorite={(project) => void toggleProjectFavorite(project)}
    onRefresh={() => void refreshRepository()}
    onSync={() => void syncRemotes()}
    onOpenSettings={() => (settingsOpen = true)}
    onViewChange={(view) => void changeNavigatorView(view)}
  />

  <div class:focus-mode={navigatorView !== 'commit'} class="workspace" style:--inspector-width={`${inspectorWidth}px`}>
    {#if navigatorView === 'commit'}
      <section class="history-pane pane">
        <HistoryToolbar
          scope={historyScope}
          allBranches={historyAllBranches}
          branches={history.branches}
          worktrees={repository?.worktrees ?? []}
          defaultBranch={repository?.default_branch ?? ''}
          currentBranch={repository?.branch ?? ''}
          currentDetached={currentWorktreeDetached}
          currentHead={repository?.head ?? ''}
          {activeProjectRoot}
          activeWorktreeRoot={repository?.root ?? ''}
          presets={appSettings.presets}
          {activePresetIDs}
          author={repository?.user ?? { name: '', email: '' }}
          disabled={!repository || historyLoading || repositoryTransitioning}
          {canEditCommits}
          {editDisabledReason}
          onScopeChange={(nextScope, nextAllBranches) => void changeHistoryScope(nextScope, nextAllBranches)}
          onWorktreeChange={(worktree) => void selectWorktree(worktree)}
          onOpenCommitEditor={() => void openCommitEditor()}
          onTogglePreset={togglePreset}
        />

        <CommitTable
          commits={history.commits}
          defaultBranch={repository?.default_branch ?? ''}
          allBranches={historyAllBranches}
          remotes={repository?.remotes ?? []}
          remoteBadgeRules={appSettings.remote_badges}
          showRemoteBadges={historyAllBranches}
          rules={filterRules}
          logic={appSettings.filter_logic}
          {selectedCommit}
          loading={historyLoading}
          loadingMore={historyLoadingMore}
          filterProgress={historyFilterProgress}
          hasMore={history.commits.length < history.total}
          autoLoad={historyAutoLoadEnabled}
          branchPoint={history.branch_point ?? ''}
          onSelect={(commit) => void selectCommit(commit)}
          onLoadMore={() => void loadMoreHistoryForCurrentFilters()}
          onSearchMessage={(message) => void addPatternSearch('msg', message)}
        />
      </section>

      <button class="pane-resizer" type="button" aria-label="Resize Inspector" title="Drag to resize Inspector" on:mousedown={startPaneResize} on:keydown={resizePaneWithKeyboard}></button>

      <Inspector
        selected={selectedForInspector}
        fileRevision={inspectorFileRevision}
        allUsesDefault={historyAllBranches}
        defaultChangedFilesView={appSettings.changed_files_view}
        remotes={repository?.remotes ?? []}
        defaultBranch={repository?.default_branch ?? ''}
        upstream={repository?.upstream ?? ''}
        onOpenIDE={(path) => void openFileInIDE(path)}
        onOpenFinder={(path) => void revealFile(path)}
        onOpenTerminal={(path) => void openInTerminal(path)}
        onOpenExternalURL={(url) => void openExternalURL(url)}
        onSelectFile={selectInspectorFile}
        onAddFileSearch={(path) => void addPatternSearch('file', path)}
        onLoadTree={loadRepositoryTree}
      />
    {:else if navigatorView === 'worktrees' && repository}
      <WorktreeGrid
        {repository}
        {activeProjectRoot}
        removing={removingWorktrees}
        onView={(worktree) => void selectWorktree(worktree)}
        onOpen={(worktree) => void openWorktree(worktree)}
        onOpenIDE={(worktree) => void openWorktreeInIDE(worktree)}
        onRemove={removeMergedWorktrees}
      />
    {:else if navigatorView === 'worktrees'}
      <section class="worktree-workspace pane"><div class="workspace-empty">Select a project to view its worktrees.</div></section>
    {:else}
      <SearchWorkspace
        sessions={searchSessionSummaries}
        activeSessionID={activeSearchSessionID}
        {repository}
        {projects}
        {activeProjectRoot}
        branches={history.branches}
        bind:patterns
        bind:engine
        bind:scope
        bind:allRefs={searchAllRefs}
        bind:author
        bind:since
        bind:until
        results={groupedSearchResults}
        bind:selectedIndex={selectedSearchIndex}
        {searching}
        {searchProgress}
        {hasSearched}
        stale={searchStale}
        applied={Boolean(executedSearchDraft)}
        {scanned}
        error={searchError}
        disabled={repositoryTransitioning}
        {inspectorWidth}
        onNewSession={() => createSearchSession()}
        onSelectSession={(id) => void selectSearchSession(id)}
        onRenameSession={renameSearchSession}
        onRemoveSession={(id) => void removeSearchSession(id)}
        onRegisterProject={() => void chooseSearchProject()}
        onSelectProject={(project) => void selectSearchProject(project)}
        onToggleProjectFavorite={(project) => void toggleProjectFavorite(project)}
        onWorktreeChange={(worktree) => void selectSearchWorktree(worktree)}
        onScopeChange={changeSearchScope}
        onRunSearch={() => void runSearch()}
        onCancelSearch={() => void cancelSearch()}
        onSelectResult={selectSearchResult}
        onStartInspectorResize={startPaneResize}
        onResizeInspectorWithKeyboard={resizePaneWithKeyboard}
      >
        <Inspector
          slot="inspector"
          selected={selectedSearchResult}
          fileRevision={selectedSearchResult?.commit ?? resultScope}
          allUsesDefault={false}
          defaultChangedFilesView={appSettings.changed_files_view}
          remotes={repository?.remotes ?? []}
          defaultBranch={repository?.default_branch ?? ''}
          upstream={repository?.upstream ?? ''}
          onOpenIDE={(path) => void openFileInIDE(path)}
          onOpenFinder={(path) => void revealFile(path)}
          onOpenTerminal={(path) => void openInTerminal(path)}
          onOpenExternalURL={(url) => void openExternalURL(url)}
          onSelectFile={selectSearchInspectorFile}
          onAddFileSearch={(path) => void addPatternSearch('file', path)}
          onLoadTree={loadRepositoryTree}
        />
      </SearchWorkspace>
    {/if}
  </div>

  <StatusBar
    repositoryOpen={Boolean(repository)}
    view={navigatorView}
    {searching}
    scope={navigatorView === 'search' && executedSearchDraft ? resultScope : navigatorView === 'search' ? scope : history.scope}
    scanned={navigatorView === 'search' ? scanned : history.total}
    count={navigatorView === 'worktrees' ? repository?.worktrees.length ?? 0 : navigatorView === 'search' ? groupedSearchResults.length : filteredCommits.length}
    loaded={history.commits.length}
    message={statusMessage}
    kind={statusKind}
    version={appVersion}
    onCancel={() => void cancelSearch()}
  />

  <SettingsModal
    open={settingsOpen}
    {projects}
    historyBatchSize={appSettings.history_batch_size}
    ide={appSettings.ide}
    terminal={appSettings.terminal}
    changedFilesView={appSettings.changed_files_view}
    presets={appSettings.presets}
    remotes={repository?.remotes ?? []}
    remoteBadgeRules={appSettings.remote_badges}
    discovering={discoveringProjects}
    {discoveryMessage}
    onClose={() => (settingsOpen = false)}
    onRegisterProject={() => void chooseRepository()}
    onDiscoverProjects={() => void discoverProjects()}
    onToggleFavorite={(project) => void toggleProjectFavorite(project)}
    onHistoryBatchSizeChange={updateHistoryBatchSize}
    onIDEChange={updateIDE}
    onTerminalChange={updateTerminal}
    onChangedFilesViewChange={updateChangedFilesView}
    onPresetsChange={updatePresets}
    onResetPresets={resetPresets}
    onRemoteBadgeRulesChange={updateRemoteBadges}
  />
  <CommitEditor
    open={commitEditorOpen}
    stack={commitEditorStack}
    loading={commitEditorLoading}
    applying={commitEditorApplying}
    error={commitEditorError}
    onClose={closeCommitEditor}
    onLoadFile={loadCommitFileContent}
    onApply={applyCommitRewrite}
  />
</main>
