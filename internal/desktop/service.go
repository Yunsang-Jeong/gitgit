package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/gitexec"
)

type Pattern struct {
	Source      string `json:"source"`
	Value       string `json:"value"`
	Join        string `json:"join,omitempty"`
	OpenGroups  int    `json:"open_groups,omitempty"`
	CloseGroups int    `json:"close_groups,omitempty"`
}

type SearchRequest struct {
	RequestID    uint64    `json:"request_id"`
	Patterns     []Pattern `json:"patterns"`
	Engine       string    `json:"engine"`
	Scope        string    `json:"scope"`
	AllRefs      bool      `json:"all_refs"`
	Author       string    `json:"author"`
	Since        string    `json:"since"`
	Until        string    `json:"until"`
	FollowRename bool      `json:"follow_rename"`
	Limit        int       `json:"limit"`
	Context      int       `json:"context"`
}

type SearchResult struct {
	Author       app.Author       `json:"author"`
	Commit       string           `json:"commit"`
	ShortCommit  string           `json:"short_commit"`
	Message      string           `json:"message"`
	Date         string           `json:"date"`
	Refs         []string         `json:"refs,omitempty"`
	File         app.FileChange   `json:"file"`
	Files        []app.FileChange `json:"files"`
	Diff         string           `json:"diff"`
	MatchSources []string         `json:"match_sources"`
}

type SearchResponse struct {
	Scope   string         `json:"scope"`
	AllRefs bool           `json:"all_refs"`
	Scanned int            `json:"scanned"`
	Count   int            `json:"count"`
	Results []SearchResult `json:"results"`
}

type HistoryRequest struct {
	Scope        string `json:"scope"`
	AllBranches  bool   `json:"all_branches"`
	RelatedScope string `json:"related_scope,omitempty"`
	Limit        int    `json:"limit"`
	Skip         int    `json:"skip"`
}

type CommitSummary struct {
	Author      app.Author       `json:"author"`
	Commit      string           `json:"commit"`
	ShortCommit string           `json:"short_commit"`
	Parents     []string         `json:"parents"`
	Message     string           `json:"message"`
	Date        string           `json:"date"`
	Refs        []string         `json:"refs,omitempty"`
	Branches    []string         `json:"branches,omitempty"`
	Files       []app.FileChange `json:"files"`
}

type HistoryResponse struct {
	Scope       string          `json:"scope"`
	AllBranches bool            `json:"all_branches"`
	Total       int             `json:"total"`
	BranchPoint string          `json:"branch_point,omitempty"`
	Branches    []string        `json:"branches"`
	Commits     []CommitSummary `json:"commits"`
}

type HistoryBranchesResponse struct {
	Branches map[string][]string `json:"branches"`
}

type RepositoryTreeEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	ObjectType string `json:"object_type"`
	OID        string `json:"oid"`
}

type RepositoryTreeResponse struct {
	Revision  string                `json:"revision"`
	Directory string                `json:"directory"`
	Entries   []RepositoryTreeEntry `json:"entries"`
}

type CommitDetail struct {
	CommitSummary
	File app.FileChange `json:"file"`
	Diff string         `json:"diff"`
}

type RepositoryState struct {
	Root          string             `json:"root"`
	ProjectRoot   string             `json:"project_root"`
	Name          string             `json:"name"`
	Branch        string             `json:"branch"`
	DefaultBranch string             `json:"default_branch"`
	User          app.Author         `json:"user"`
	Upstream      string             `json:"upstream,omitempty"`
	Head          string             `json:"head,omitempty"`
	Dirty         bool               `json:"dirty"`
	Ahead         int                `json:"ahead"`
	Behind        int                `json:"behind"`
	Worktrees     []app.WorktreeInfo `json:"worktrees"`
	Remotes       []RemoteInfo       `json:"remotes"`
}

type RemoteInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RemoteSyncResult struct {
	State    RepositoryState `json:"state"`
	Warnings []string        `json:"warnings,omitempty"`
}

type Service struct {
	runner    *gitexec.Runner
	rewriteMu sync.Mutex

	mu                   sync.RWMutex
	repository           *gitexec.Repository
	switchingRepository  bool
	repositoryGeneration uint64
	operations           map[uint64]context.CancelFunc
	latestOperations     map[string]uint64
	nextOperationID      uint64
	searchCancel         context.CancelFunc
	searchID             uint64
	cacheMu              sync.Mutex
	caches               map[string]*repositoryCache
	cacheRoots           []string
	persistentCache      *PersistentCache
}

func NewService(runner *gitexec.Runner) *Service {
	return NewServiceWithCache(runner, nil)
}

func NewServiceWithCache(runner *gitexec.Runner, persistentCache *PersistentCache) *Service {
	if runner == nil {
		runner = gitexec.NewRunner()
	}
	return &Service{
		runner:           runner,
		operations:       make(map[uint64]context.CancelFunc),
		latestOperations: make(map[string]uint64),
		caches:           make(map[string]*repositoryCache),
		persistentCache:  persistentCache,
	}
}

func (s *Service) Close() error {
	s.CancelOperations()
	if s.persistentCache == nil {
		return nil
	}
	return s.persistentCache.Close()
}

func (s *Service) Open(ctx context.Context, path string) (RepositoryState, error) {
	generation := s.reserveRepositorySwitch()
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish, current := s.beginReservedRepositorySwitch(ctx, generation)
	if !current {
		return RepositoryState{}, context.Canceled
	}
	completed := false
	defer func() {
		finish()
		if !completed {
			s.failRepositorySwitch(generation)
		}
	}()
	repository, err := gitexec.OpenRepository(operationContext, s.runner, path)
	if err != nil {
		return RepositoryState{}, err
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return RepositoryState{}, err
	}
	if !s.completeRepositorySwitch(generation, repository) {
		return RepositoryState{}, context.Canceled
	}
	completed = true
	return state, nil
}

func (s *Service) Current(ctx context.Context) (RepositoryState, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return RepositoryState{}, err
	}
	return s.snapshot(operationContext, repository)
}

func (s *Service) SyncRemotes(ctx context.Context) (RemoteSyncResult, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return RemoteSyncResult{}, err
	}
	remoteOutput, err := repository.Run(operationContext, nil, "remote")
	if err != nil {
		return RemoteSyncResult{}, fmt.Errorf("list remotes: %w", err)
	}
	if len(strings.Fields(string(remoteOutput))) == 0 {
		state, snapshotErr := s.snapshot(operationContext, repository)
		if snapshotErr != nil {
			return RemoteSyncResult{}, snapshotErr
		}
		return RemoteSyncResult{State: state, Warnings: []string{"No remotes configured; local state refreshed."}}, nil
	}
	if _, err := repository.Run(operationContext, nil, "fetch", "--all", "--prune"); err != nil {
		return RemoteSyncResult{}, fmt.Errorf("fetch all remotes: %w", err)
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return RemoteSyncResult{}, err
	}
	return RemoteSyncResult{State: state}, nil
}

type removableWorktree struct {
	info       app.WorktreeInfo
	branchRef  string
	branchHead string
}

func (s *Service) RemoveMergedWorktree(ctx context.Context, path string) (RepositoryState, error) {
	return s.RemoveMergedWorktrees(ctx, []string{path})
}

func (s *Service) RemoveMergedWorktrees(ctx context.Context, paths []string) (RepositoryState, error) {
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return RepositoryState{}, err
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return RepositoryState{}, err
	}

	if len(paths) == 0 {
		return RepositoryState{}, errors.New("select at least one worktree to remove")
	}
	if state.DefaultBranch == "" {
		return RepositoryState{}, errors.New("the default branch could not be resolved")
	}
	defaultRef := "refs/heads/" + state.DefaultBranch
	defaultHead, err := resolveCommit(operationContext, repository, defaultRef)
	if err != nil {
		return RepositoryState{}, fmt.Errorf("resolve default branch %s: %w", state.DefaultBranch, err)
	}

	worktreesByPath := make(map[string]app.WorktreeInfo, len(state.Worktrees))
	for _, worktree := range state.Worktrees {
		worktreesByPath[canonicalWorktreePath(worktree.Path)] = worktree
	}
	seen := make(map[string]struct{}, len(paths))
	targets := make([]removableWorktree, 0, len(paths))
	for _, requestedPath := range paths {
		path := canonicalWorktreePath(requestedPath)
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}

		target, ok := worktreesByPath[path]
		if !ok {
			return RepositoryState{}, fmt.Errorf("worktree is not registered: %s", requestedPath)
		}
		if canonicalWorktreePath(target.Path) == canonicalWorktreePath(state.ProjectRoot) {
			return RepositoryState{}, errors.New("the main worktree cannot be removed")
		}
		if canonicalWorktreePath(target.Path) == canonicalWorktreePath(state.Root) {
			return RepositoryState{}, errors.New("the currently viewed worktree cannot be removed; switch to Main first")
		}
		if target.Detached || target.Branch == "" {
			return RepositoryState{}, errors.New("a detached worktree branch cannot be removed by this action")
		}
		if target.Locked {
			return RepositoryState{}, fmt.Errorf("unlock worktree %s before removing it", target.Branch)
		}
		if target.Branch == state.DefaultBranch {
			return RepositoryState{}, errors.New("the default branch worktree cannot be removed")
		}
		if target.Dirty {
			return RepositoryState{}, fmt.Errorf("worktree %s has local changes", target.Branch)
		}
		if !target.MergedIntoDefault {
			return RepositoryState{}, fmt.Errorf("branch %s is not merged into %s", target.Branch, state.DefaultBranch)
		}

		status, statusErr := repository.Runner.Run(operationContext, target.Path, nil, "status", "--porcelain=v2", "-z")
		if statusErr != nil {
			return RepositoryState{}, fmt.Errorf("verify worktree %s status: %w", target.Branch, statusErr)
		}
		if len(status) > 0 {
			return RepositoryState{}, fmt.Errorf("worktree %s changed while removal was being prepared", target.Branch)
		}

		branchRef := "refs/heads/" + target.Branch
		branchHead, branchErr := resolveCommit(operationContext, repository, branchRef)
		if branchErr != nil {
			return RepositoryState{}, fmt.Errorf("verify worktree branch %s: %w", target.Branch, branchErr)
		}
		if branchHead == "" || branchHead != target.Head {
			return RepositoryState{}, fmt.Errorf("worktree branch %s changed while removal was being prepared", target.Branch)
		}
		if _, mergeErr := repository.Run(operationContext, nil, "merge-base", "--is-ancestor", branchHead, defaultHead); mergeErr != nil {
			return RepositoryState{}, fmt.Errorf("branch %s is no longer merged into %s", target.Branch, state.DefaultBranch)
		}
		targets = append(targets, removableWorktree{info: target, branchRef: branchRef, branchHead: branchHead})
	}

	worktreeService := app.NewWorktreeService(repository)
	for index, target := range targets {
		if _, removeErr := worktreeService.Remove(operationContext, target.info.Path, true); removeErr != nil {
			return RepositoryState{}, fmt.Errorf("removed %d of %d worktrees; remove %s: %w", index, len(targets), target.info.Branch, removeErr)
		}
		if branchErr := deleteBranchIfDefaultUnchanged(operationContext, repository, defaultRef, defaultHead, target.branchRef, target.branchHead); branchErr != nil {
			return RepositoryState{}, fmt.Errorf("worktree was removed but branch %s was retained: %w", target.info.Branch, branchErr)
		}
		_, _ = repository.Run(operationContext, nil, "config", "--remove-section", "branch."+target.info.Branch)
	}

	return s.snapshot(operationContext, repository)
}

func deleteBranchIfDefaultUnchanged(ctx context.Context, repository *gitexec.Repository, defaultRef, defaultHead, branchRef, branchHead string) error {
	commands := fmt.Sprintf(
		"start\nverify %s %s\ndelete %s %s\nprepare\ncommit\n",
		defaultRef, defaultHead, branchRef, branchHead,
	)
	if _, err := repository.Run(ctx, strings.NewReader(commands), "update-ref", "--stdin"); err != nil {
		return fmt.Errorf("verify default branch and delete branch atomically: %w", err)
	}
	return nil
}

func canonicalWorktreePath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if absolute, err := filepath.Abs(path); err == nil {
		path = filepath.Clean(absolute)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(resolved)
	}
	return path
}

func (s *Service) History(ctx context.Context, request HistoryRequest) (HistoryResponse, error) {
	operationContext, finish := s.beginLatestOperation(ctx, "history")
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return HistoryResponse{}, err
	}
	scope, all := normalizeHistoryScope(request.Scope, request.AllBranches)
	if !all {
		if err := validateRevisionExpression("history scope", scope); err != nil {
			return HistoryResponse{}, err
		}
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	request.Limit = limit
	if request.Skip < 0 {
		return HistoryResponse{}, errors.New("history skip must be zero or greater")
	}

	relatedScope := normalizeRelatedScope(request.RelatedScope, scope)
	if relatedScope != "" {
		if err := validateRevisionExpression("related history scope", relatedScope); err != nil {
			return HistoryResponse{}, err
		}
	}
	fingerprint, err := repositoryFingerprint(operationContext, repository)
	if err != nil {
		return HistoryResponse{}, err
	}
	headIdentity := ""
	if !all && (scope == "HEAD" || relatedScope == "HEAD") {
		head, headErr := repository.Run(operationContext, nil, "rev-parse", "--verify", "HEAD")
		if headErr != nil {
			return HistoryResponse{}, fmt.Errorf("resolve history HEAD: %w", headErr)
		}
		headIdentity = strings.TrimSpace(string(head))
	}
	scopeCacheKey := historyScopeCacheKey(repository.Root, scope, relatedScope, headIdentity, all)
	scopeContext, cachedScope := s.cachedHistoryScope(repository.CommonDir, fingerprint, scopeCacheKey)
	if !cachedScope {
		branchPoint, branchPointParents, branchErr := historyBranchPoint(operationContext, repository, scope, relatedScope)
		if branchErr != nil {
			return HistoryResponse{}, branchErr
		}
		if relatedScope != "" && branchPoint == "" {
			relatedScope = ""
		}
		allRevisions := []string(nil)
		allowedRemoteRefs := map[string]bool(nil)
		if all {
			allRevisions, allowedRemoteRefs, err = historyAllRevisions(operationContext, repository)
			if err != nil {
				return HistoryResponse{}, fmt.Errorf("resolve All branches history refs: %w", err)
			}
		}
		total, countErr := countHistoryCommits(operationContext, repository, scope, relatedScope, allRevisions)
		if countErr != nil {
			return HistoryResponse{}, fmt.Errorf("count commit history: %w", countErr)
		}
		branches, branchErr := readBranches(operationContext, repository)
		if branchErr != nil {
			return HistoryResponse{}, fmt.Errorf("list branches: %w", branchErr)
		}
		scopeContext = historyScopeContext{
			relatedScope: relatedScope, branchPoint: branchPoint, branchPointParents: branchPointParents,
			allRevisions: allRevisions, allowedRemoteRefs: allowedRemoteRefs, total: total, branches: branches,
		}
		s.storeHistoryScope(repository.CommonDir, fingerprint, scopeCacheKey, scopeContext)
	}
	relatedScope = scopeContext.relatedScope
	branchPoint := scopeContext.branchPoint
	branchPointParents := scopeContext.branchPointParents
	allRevisions := scopeContext.allRevisions
	allowedRemoteRefs := scopeContext.allowedRemoteRefs
	cacheKey := historyCacheKey(request, scope, relatedScope, headIdentity, all, allRevisions)
	if cached, ok := s.cachedHistory(repository.CommonDir, fingerprint, cacheKey); ok {
		return cached, nil
	}

	args := []string{
		"log", "--topo-order", "-z", "--date=iso-strict",
		"--format=%H%x00%P%x00%an%x00%ae%x00%aI%x00%B%x00%D",
	}
	if request.Skip > 0 || branchPoint == "" {
		args = append(args, fmt.Sprintf("--max-count=%d", limit), fmt.Sprintf("--skip=%d", request.Skip))
	}
	if all {
		args = append(args, "--end-of-options")
		args = append(args, allRevisions...)
	} else {
		args = append(args, "--end-of-options", scope)
		if relatedScope != "" {
			args = append(args, relatedScope)
		}
		if request.Skip == 0 && branchPoint != "" {
			for _, parent := range branchPointParents {
				args = append(args, "^"+parent)
			}
		}
	}
	out, err := repository.Run(operationContext, nil, args...)
	if err != nil {
		return HistoryResponse{}, fmt.Errorf("list commit history: %w", err)
	}
	commits, err := parseHistoryLog(out)
	if err != nil {
		return HistoryResponse{}, err
	}
	if all {
		filterRemoteDecorations(commits, allowedRemoteRefs)
	}
	commitOIDs := make([]string, len(commits))
	for index := range commits {
		commitOIDs[index] = commits[index].Commit
	}
	filesByCommit, err := app.ReadCommitFileChangesBatch(operationContext, repository, commitOIDs, nil, true)
	if err != nil {
		return HistoryResponse{}, fmt.Errorf("read history changed files: %w", err)
	}
	for index := range commits {
		commits[index].Files = filesByCommit[commits[index].Commit]
	}
	response := HistoryResponse{
		Scope: scope, AllBranches: all, Total: scopeContext.total,
		BranchPoint: branchPoint, Branches: scopeContext.branches, Commits: commits,
	}
	s.storeHistory(repository.CommonDir, fingerprint, cacheKey, response)
	return response, nil
}

func historyBranchPoint(ctx context.Context, repository *gitexec.Repository, scope, relatedScope string) (string, []string, error) {
	if relatedScope == "" {
		return "", nil, nil
	}
	out, err := repository.Run(ctx, nil, "merge-base", "--end-of-options", scope, relatedScope)
	if err != nil {
		return "", nil, nil
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", nil, nil
	}
	branchPoint := fields[0]
	parentsOutput, err := repository.Run(ctx, nil, "show", "-s", "--format=%P", "--end-of-options", branchPoint)
	if err != nil {
		return "", nil, fmt.Errorf("read branch point parents: %w", err)
	}
	return branchPoint, strings.Fields(string(parentsOutput)), nil
}

func (s *Service) HistoryBranches(ctx context.Context, commits []string) (HistoryBranchesResponse, error) {
	operationContext, finish := s.beginLatestOperation(ctx, "history-branches")
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return HistoryBranchesResponse{}, err
	}
	fingerprint, err := repositoryFingerprint(operationContext, repository)
	if err != nil {
		return HistoryBranchesResponse{}, err
	}

	response := HistoryBranchesResponse{Branches: make(map[string][]string)}
	missing := make([]string, 0, len(commits))
	seen := make(map[string]bool, len(commits))
	for _, oid := range commits {
		oid = strings.TrimSpace(oid)
		if oid == "" || seen[oid] {
			continue
		}
		seen[oid] = true
		if branches, ok := s.cachedBranches(repository.CommonDir, fingerprint, oid); ok {
			response.Branches[oid] = branches
			continue
		}
		missing = append(missing, oid)
	}
	if len(missing) == 0 {
		return response, nil
	}

	type branchResult struct {
		oid      string
		branches []string
		err      error
	}
	workerCount := min(4, len(missing))
	jobs := make(chan string, len(missing))
	results := make(chan branchResult, len(missing))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for oid := range jobs {
				branches, branchErr := readContainingBranches(operationContext, repository, oid)
				results <- branchResult{oid: oid, branches: branches, err: branchErr}
			}
		}()
	}
	for _, oid := range missing {
		jobs <- oid
	}
	close(jobs)
	go func() {
		workers.Wait()
		close(results)
	}()
	for result := range results {
		if result.err != nil {
			return HistoryBranchesResponse{}, result.err
		}
		response.Branches[result.oid] = result.branches
		s.storeBranches(repository.CommonDir, fingerprint, result.oid, result.branches)
	}
	return response, nil
}

func (s *Service) CommitDetail(ctx context.Context, oid, filePath string) (CommitDetail, error) {
	operationContext, finish := s.beginLatestOperation(ctx, "commit-detail")
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return CommitDetail{}, err
	}
	oid = strings.TrimSpace(oid)
	if oid == "" {
		return CommitDetail{}, errors.New("commit is required")
	}
	if err := validateRevisionExpression("commit", oid); err != nil {
		return CommitDetail{}, err
	}
	resolved, err := repository.Run(operationContext, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", oid+"^{commit}")
	if err != nil {
		return CommitDetail{}, fmt.Errorf("resolve commit: %w", err)
	}
	resolvedOID := strings.TrimSpace(string(resolved))
	cacheKey := detailCacheKey(resolvedOID, filePath)
	if cached, ok := s.cachedDetail(repository.CommonDir, cacheKey); ok {
		return cached, nil
	}
	summary, err := readCommitSummary(operationContext, repository, resolvedOID)
	if err != nil {
		return CommitDetail{}, err
	}
	if len(summary.Files) == 0 {
		detail := CommitDetail{CommitSummary: summary}
		s.storeDetail(repository.CommonDir, cacheKey, detail)
		return detail, nil
	}
	selected := summary.Files[0]
	if filePath != "" {
		found := false
		for _, file := range summary.Files {
			if file.Path == filePath || file.OldPath == filePath {
				selected = file
				found = true
				break
			}
		}
		if !found {
			return CommitDetail{}, fmt.Errorf("file %q is not changed by commit %s", filePath, summary.ShortCommit)
		}
	}
	diff, err := readCommitDiff(operationContext, repository, resolvedOID, selected)
	if err != nil {
		return CommitDetail{}, err
	}
	detail := CommitDetail{CommitSummary: summary, File: selected, Diff: diff}
	s.storeDetail(repository.CommonDir, cacheKey, detail)
	return detail, nil
}

func (s *Service) RepositoryTree(ctx context.Context, revision, directory string) (RepositoryTreeResponse, error) {
	// A single tree view may expand several sibling directories concurrently.
	// Keep those reads independent; repository switches still cancel them all.
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return RepositoryTreeResponse{}, err
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return RepositoryTreeResponse{}, errors.New("revision is required")
	}
	if err := validateRevisionExpression("revision", revision); err != nil {
		return RepositoryTreeResponse{}, err
	}
	directory, err = normalizeTreeDirectory(directory)
	if err != nil {
		return RepositoryTreeResponse{}, err
	}

	treeish := revision + "^{tree}"
	if directory != "" {
		treeish = revision + ":" + directory
	}
	resolved, err := repository.Run(operationContext, nil, "rev-parse", "--verify", "--quiet", "--end-of-options", treeish)
	if err != nil {
		return RepositoryTreeResponse{}, fmt.Errorf("resolve repository tree %q: %w", treeish, err)
	}
	treeOID := strings.TrimSpace(string(resolved))
	objectType, err := repository.Run(operationContext, nil, "cat-file", "-t", treeOID)
	if err != nil {
		return RepositoryTreeResponse{}, fmt.Errorf("inspect repository tree %q: %w", treeish, err)
	}
	if strings.TrimSpace(string(objectType)) != "tree" {
		return RepositoryTreeResponse{}, fmt.Errorf("repository path %q is not a directory", directory)
	}

	out, err := repository.Run(operationContext, nil, "ls-tree", "-z", treeOID)
	if err != nil {
		return RepositoryTreeResponse{}, fmt.Errorf("list repository tree %q: %w", treeish, err)
	}
	entries, err := parseRepositoryTree(out, directory)
	if err != nil {
		return RepositoryTreeResponse{}, err
	}
	return RepositoryTreeResponse{Revision: revision, Directory: directory, Entries: entries}, nil
}

func normalizeTreeDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "", nil
	}
	if strings.ContainsRune(value, 0) || pathpkg.IsAbs(value) {
		return "", errors.New("repository tree directory must be relative")
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("repository tree directory cannot escape the repository")
	}
	return cleaned, nil
}

func parseRepositoryTree(data []byte, directory string) ([]RepositoryTreeEntry, error) {
	records := bytes.Split(data, []byte{0})
	entries := make([]RepositoryTreeEntry, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		metadata, name, found := bytes.Cut(record, []byte{'\t'})
		if !found || len(name) == 0 {
			return nil, fmt.Errorf("invalid repository tree record %q", string(record))
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid repository tree metadata %q", string(metadata))
		}
		entryName := string(name)
		entryPath := entryName
		if directory != "" {
			entryPath = pathpkg.Join(directory, entryName)
		}
		entries = append(entries, RepositoryTreeEntry{
			Name: entryName, Path: entryPath, ObjectType: fields[1], OID: fields[2],
		})
	}
	return entries, nil
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	return s.SearchWithProgress(ctx, request, nil)
}

func (s *Service) SearchWithProgress(ctx context.Context, request SearchRequest, onProgress func(app.SearchProgress)) (SearchResponse, error) {
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return SearchResponse{}, err
	}
	options, scope, err := searchOptions(request)
	if err != nil {
		return SearchResponse{}, err
	}

	searchContext, cancel := context.WithCancel(operationContext)
	s.mu.Lock()
	if s.searchCancel != nil {
		s.searchCancel()
	}
	s.searchID++
	searchID := s.searchID
	s.searchCancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		if s.searchID == searchID {
			s.searchCancel = nil
		}
		s.mu.Unlock()
	}()

	response, err := app.NewSearchService(repository).SearchWithProgress(searchContext, options, onProgress)
	if err != nil {
		return SearchResponse{}, err
	}

	results := make([]SearchResult, 0, len(response.Results))
	for _, match := range response.Results {
		results = append(results, SearchResult{
			Author:       match.Author,
			Commit:       match.Commit,
			ShortCommit:  shortOID(match.Commit),
			Message:      match.Message,
			Date:         match.Date,
			Refs:         match.Refs,
			File:         match.File,
			Files:        match.Files,
			Diff:         match.Diff,
			MatchSources: match.MatchSources,
		})
	}
	return SearchResponse{Scope: scope, AllRefs: options.All, Scanned: response.Scanned, Count: len(results), Results: results}, nil
}

func (s *Service) CancelSearch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.searchCancel != nil {
		s.searchCancel()
		s.searchCancel = nil
	}
	s.searchID++
}

func (s *Service) currentRepository() (*gitexec.Repository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.switchingRepository {
		return nil, errors.New("repository switch is in progress")
	}
	if s.repository == nil {
		return nil, errors.New("no repository is open")
	}
	return s.repository, nil
}

func (s *Service) snapshot(ctx context.Context, repository *gitexec.Repository) (RepositoryState, error) {
	state := RepositoryState{Root: repository.Root, Name: filepath.Base(repository.Root)}
	if out, err := repository.Run(ctx, nil, "config", "--get", "user.name"); err == nil {
		state.User.Name = strings.TrimSpace(string(out))
	}
	if out, err := repository.Run(ctx, nil, "config", "--get", "user.email"); err == nil {
		state.User.Email = strings.TrimSpace(string(out))
	}
	if out, err := repository.Run(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		state.Branch = strings.TrimSpace(string(out))
	} else {
		state.Branch = "detached"
	}
	if out, err := repository.Run(ctx, nil, "rev-parse", "HEAD"); err == nil {
		state.Head = strings.TrimSpace(string(out))
	}
	status, err := repository.Run(ctx, nil, "status", "--porcelain=v2", "-z")
	if err != nil {
		return RepositoryState{}, fmt.Errorf("read repository status: %w", err)
	}
	state.Dirty = len(status) > 0
	if out, err := repository.Run(ctx, nil, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		state.Upstream = strings.TrimSpace(string(out))
		if counts, countErr := repository.Run(ctx, nil, "rev-list", "--left-right", "--count", "HEAD...@{upstream}"); countErr == nil {
			fields := strings.Fields(string(counts))
			if len(fields) == 2 {
				state.Ahead, _ = strconv.Atoi(fields[0])
				state.Behind, _ = strconv.Atoi(fields[1])
			}
		}
	}
	state.Worktrees, err = app.NewWorktreeService(repository).List(ctx)
	if err != nil {
		return RepositoryState{}, err
	}
	state.ProjectRoot = state.Root
	if len(state.Worktrees) > 0 {
		// Git always emits the main worktree first, even when the repository was
		// opened through a linked worktree.
		state.ProjectRoot = state.Worktrees[0].Path
	}
	state.DefaultBranch = readDefaultBranch(ctx, repository, state.Branch)
	state.Remotes = readRemotes(ctx, repository)
	mergedBranches := readMergedBranches(ctx, repository, state.DefaultBranch)
	for index := range state.Worktrees {
		state.Worktrees[index].MergedIntoDefault = mergedBranches[state.Worktrees[index].Branch]
	}
	return state, nil
}

func searchOptions(request SearchRequest) (app.SearchOptions, string, error) {
	options := app.SearchOptions{
		Engine:         strings.ToLower(strings.TrimSpace(request.Engine)),
		Author:         strings.TrimSpace(request.Author),
		Since:          strings.TrimSpace(request.Since),
		Until:          strings.TrimSpace(request.Until),
		FollowRename:   request.FollowRename,
		Limit:          request.Limit,
		Context:        request.Context,
		OmitResultDiff: true,
	}
	if options.Engine == "" {
		options.Engine = "glob"
	}
	if options.Limit <= 0 {
		options.Limit = 100
	}
	if options.Context < 0 {
		return app.SearchOptions{}, "", errors.New("context must be zero or greater")
	}
	if options.Context == 0 {
		options.Context = 3
	}
	for _, pattern := range request.Patterns {
		value := strings.TrimSpace(pattern.Value)
		if value == "" {
			continue
		}
		source := strings.ToLower(strings.TrimSpace(pattern.Source))
		switch source {
		case "msg":
			options.Messages = append(options.Messages, value)
		case "diff":
			options.Diffs = append(options.Diffs, value)
		case "file":
			options.Files = append(options.Files, value)
		default:
			return app.SearchOptions{}, "", fmt.Errorf("unsupported pattern source %q", pattern.Source)
		}
		join := strings.ToLower(strings.TrimSpace(pattern.Join))
		if len(options.Predicates) == 0 {
			join = ""
		} else if join == "" {
			join = "or"
		} else if join != "and" && join != "or" {
			return app.SearchOptions{}, "", fmt.Errorf("unsupported search join %q", pattern.Join)
		}
		options.Predicates = append(options.Predicates, app.SearchPredicate{
			Source: source, Value: value, Join: join,
			OpenGroups: pattern.OpenGroups, CloseGroups: pattern.CloseGroups,
		})
	}
	if len(options.Messages)+len(options.Diffs)+len(options.Files) == 0 {
		return app.SearchOptions{}, "", errors.New("add at least one message, diff, or file pattern")
	}
	scope := strings.TrimSpace(request.Scope)
	if request.AllRefs {
		options.All = true
		scope = "All refs"
	} else if scope == "" || scope == "HEAD" {
		scope = "HEAD"
	} else {
		if err := validateRevisionExpression("search scope", scope); err != nil {
			return app.SearchOptions{}, "", err
		}
		options.Revision = scope
	}
	return options, scope, nil
}

func normalizeHistoryScope(value string, allBranches bool) (string, bool) {
	if allBranches {
		return "All branches", true
	}
	scope := strings.TrimSpace(value)
	switch scope {
	case "", "HEAD":
		return "HEAD", false
	default:
		return scope, false
	}
}

func validateRevisionExpression(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s cannot be a Git option", label)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("%s contains an invalid NUL byte", label)
	}
	return nil
}

func parseHistoryLog(data []byte) ([]CommitSummary, error) {
	fields := bytes.Split(data, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	if len(fields)%7 != 0 {
		return nil, fmt.Errorf("unexpected history field count %d", len(fields))
	}
	commits := make([]CommitSummary, 0, len(fields)/7)
	for index := 0; index < len(fields); index += 7 {
		oid := string(fields[index])
		commits = append(commits, CommitSummary{
			Commit:      oid,
			ShortCommit: shortOID(oid),
			Parents:     parseCommitParents(fields[index+1]),
			Author: app.Author{
				Name:  string(fields[index+2]),
				Email: string(fields[index+3]),
			},
			Date:    string(fields[index+4]),
			Message: strings.TrimSpace(string(fields[index+5])),
			Refs:    parseDecorations(string(fields[index+6])),
		})
	}
	return commits, nil
}

func readCommitSummary(ctx context.Context, repository *gitexec.Repository, oid string) (CommitSummary, error) {
	out, err := repository.Run(ctx, nil, "show", "-s", "--date=iso-strict", "--format=%H%x00%P%x00%an%x00%ae%x00%aI%x00%B%x00%D", oid)
	if err != nil {
		return CommitSummary{}, fmt.Errorf("read commit metadata: %w", err)
	}
	fields := bytes.SplitN(bytes.TrimRight(out, "\n"), []byte{0}, 7)
	if len(fields) != 7 {
		return CommitSummary{}, fmt.Errorf("unexpected commit field count %d", len(fields))
	}
	resolvedOID := string(fields[0])
	files, err := readCommitFiles(ctx, repository, resolvedOID)
	if err != nil {
		return CommitSummary{}, fmt.Errorf("read changed files: %w", err)
	}
	return CommitSummary{
		Commit:      resolvedOID,
		ShortCommit: shortOID(resolvedOID),
		Parents:     parseCommitParents(fields[1]),
		Author:      app.Author{Name: string(fields[2]), Email: string(fields[3])},
		Date:        string(fields[4]),
		Message:     strings.TrimSpace(string(fields[5])),
		Refs:        parseDecorations(string(fields[6])),
		Files:       files,
	}, nil
}

func parseCommitParents(value []byte) []string {
	parents := strings.Fields(string(value))
	if parents == nil {
		return []string{}
	}
	return parents
}

func parseDecorations(value string) []string {
	refs := make([]string, 0)
	for _, ref := range strings.Split(value, ",") {
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "HEAD -> ")
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

// historyAllRevisions keeps the default Commit view focused on refs the user
// owns locally. A remote contributes only the branch whose name matches the
// local default branch; explicit Search "All refs" continues to use git --all.
func historyAllRevisions(ctx context.Context, repository *gitexec.Repository) ([]string, map[string]bool, error) {
	localOutput, err := repository.Run(ctx, nil, "for-each-ref", "--format=%(refname)", "refs/heads")
	if err != nil {
		return nil, nil, err
	}
	remoteOutput, err := repository.Run(ctx, nil, "for-each-ref", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return nil, nil, err
	}

	current := ""
	if out, currentErr := repository.Run(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD"); currentErr == nil {
		current = strings.TrimSpace(string(out))
	}
	defaultBranch := readDefaultBranch(ctx, repository, current)
	remoteNames := make([]string, 0)
	for _, remote := range readRemotes(ctx, repository) {
		remoteNames = append(remoteNames, remote.Name)
	}
	sort.Slice(remoteNames, func(left, right int) bool { return len(remoteNames[left]) > len(remoteNames[right]) })
	if _, branch, ok := splitRemoteRef(defaultBranch, remoteNames); ok {
		defaultBranch = branch
	}

	revisions := make([]string, 0)
	for _, ref := range strings.Fields(string(localOutput)) {
		if strings.HasPrefix(ref, "refs/heads/") {
			revisions = append(revisions, ref)
		}
	}
	remoteDecorations := make(map[string]bool)
	for _, ref := range strings.Fields(string(remoteOutput)) {
		shortRef := strings.TrimPrefix(ref, "refs/remotes/")
		_, branch, ok := splitRemoteRef(shortRef, remoteNames)
		if !ok {
			continue
		}
		included := defaultBranch != "" && defaultBranch != "HEAD" && branch == defaultBranch
		remoteDecorations[shortRef] = included
		if included {
			revisions = append(revisions, ref)
		}
	}
	if len(revisions) == 0 {
		revisions = append(revisions, "HEAD")
	}
	sort.Strings(revisions)
	return revisions, remoteDecorations, nil
}

func splitRemoteRef(ref string, remoteNames []string) (string, string, bool) {
	for _, remote := range remoteNames {
		prefix := remote + "/"
		if strings.HasPrefix(ref, prefix) {
			return remote, strings.TrimPrefix(ref, prefix), true
		}
	}
	return "", "", false
}

func filterRemoteDecorations(commits []CommitSummary, remoteDecorations map[string]bool) {
	for index := range commits {
		refs := commits[index].Refs[:0]
		for _, ref := range commits[index].Refs {
			if included, remote := remoteDecorations[ref]; remote && !included {
				continue
			}
			refs = append(refs, ref)
		}
		commits[index].Refs = refs
	}
}

func countHistoryCommits(ctx context.Context, repository *gitexec.Repository, scope, relatedScope string, allRevisions []string) (int, error) {
	args := []string{"rev-list", "--count"}
	if len(allRevisions) > 0 {
		args = append(args, "--end-of-options")
		args = append(args, allRevisions...)
	} else {
		args = append(args, "--end-of-options", scope)
		if relatedScope != "" {
			args = append(args, relatedScope)
		}
	}
	out, err := repository.Run(ctx, nil, args...)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

func normalizeRelatedScope(value, scope string) string {
	related := strings.TrimSpace(value)
	if related == "" || strings.EqualFold(related, scope) {
		return ""
	}
	return related
}

func readDefaultBranch(ctx context.Context, repository *gitexec.Repository, _ string) string {
	if out, err := repository.Run(ctx, nil, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		remoteBranch := strings.TrimSpace(string(out))
		localBranch := strings.TrimPrefix(remoteBranch, "origin/")
		if _, verifyErr := repository.Run(ctx, nil, "show-ref", "--verify", "--quiet", "refs/heads/"+localBranch); verifyErr == nil {
			return localBranch
		}
		if remoteBranch != "" {
			return remoteBranch
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if _, err := repository.Run(ctx, nil, "show-ref", "--verify", "--quiet", "refs/heads/"+candidate); err == nil {
			return candidate
		}
	}
	if repository.CommonDir != "" {
		out, err := repository.Runner.Run(ctx, repository.Root, nil, "--git-dir="+repository.CommonDir, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err == nil {
			branch := strings.TrimSpace(string(out))
			if branch != "" {
				return branch
			}
		}
	}
	out, err := repository.Run(ctx, nil, "for-each-ref", "--count=1", "--format=%(refname:short)", "refs/heads")
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" {
			return branch
		}
	}
	return "HEAD"
}

func readMergedBranches(ctx context.Context, repository *gitexec.Repository, defaultBranch string) map[string]bool {
	merged := map[string]bool{}
	if defaultBranch == "" || defaultBranch == "HEAD" {
		return merged
	}
	out, err := repository.Run(ctx, nil, "for-each-ref", "--format=%(refname:short)", "--merged="+defaultBranch, "refs/heads")
	if err != nil {
		return merged
	}
	for _, branch := range strings.Split(string(out), "\n") {
		branch = strings.TrimSpace(branch)
		if branch != "" {
			merged[branch] = true
		}
	}
	return merged
}

func readBranches(ctx context.Context, repository *gitexec.Repository) ([]string, error) {
	out, err := repository.Run(ctx, nil, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	branches := make([]string, 0)
	for _, branch := range strings.Split(string(out), "\n") {
		branch = strings.TrimSpace(branch)
		if branch == "" || strings.HasSuffix(branch, "/HEAD") || seen[branch] {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches, nil
}

func readRemotes(ctx context.Context, repository *gitexec.Repository) []RemoteInfo {
	out, err := repository.Run(ctx, nil, "remote")
	if err != nil {
		return nil
	}
	remotes := make([]RemoteInfo, 0)
	for _, name := range strings.Fields(string(out)) {
		urlOutput, urlErr := repository.Run(ctx, nil, "remote", "get-url", name)
		if urlErr != nil {
			continue
		}
		remotes = append(remotes, RemoteInfo{Name: name, URL: strings.TrimSpace(string(urlOutput))})
	}
	sort.Slice(remotes, func(left, right int) bool { return remotes[left].Name < remotes[right].Name })
	return remotes
}

func readContainingBranches(ctx context.Context, repository *gitexec.Repository, oid string) ([]string, error) {
	out, err := repository.Run(ctx, nil, "branch", "--contains", oid, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	branches := make([]string, 0)
	for _, branch := range strings.Split(string(out), "\n") {
		branch = strings.TrimSpace(branch)
		if branch == "" || strings.HasSuffix(branch, "/HEAD") || seen[branch] {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches, nil
}

func readCommitDiff(ctx context.Context, repository *gitexec.Repository, oid string, file app.FileChange) (string, error) {
	args := []string{"show", "--first-parent", "--format=", "--no-ext-diff", "--find-renames", "--unified=3", oid, "--"}
	if file.OldPath != "" {
		args = append(args, file.OldPath)
	}
	args = append(args, file.Path)
	out, err := repository.Run(ctx, nil, args...)
	if err != nil {
		return "", fmt.Errorf("read commit diff: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func readCommitFiles(ctx context.Context, repository *gitexec.Repository, oid string) ([]app.FileChange, error) {
	out, err := repository.Run(ctx, nil, "show", "--first-parent", "--format=", "--name-status", "-r", "-z", "-M", oid, "--")
	if err != nil {
		return nil, err
	}
	tokens := bytes.Split(out, []byte{0})
	changes := make([]app.FileChange, 0, len(tokens)/2)
	for index := 0; index < len(tokens); {
		status := string(tokens[index])
		index++
		if status == "" {
			continue
		}
		if index >= len(tokens) {
			return nil, fmt.Errorf("missing path for status %q", status)
		}
		if status[0] == 'R' || status[0] == 'C' {
			if index+1 >= len(tokens) {
				return nil, fmt.Errorf("missing rename paths for status %q", status)
			}
			changes = append(changes, app.FileChange{Status: status, OldPath: string(tokens[index]), Path: string(tokens[index+1])})
			index += 2
			continue
		}
		changes = append(changes, app.FileChange{Status: status, Path: string(tokens[index])})
		index++
	}
	return changes, nil
}

func shortOID(oid string) string {
	if len(oid) <= 8 {
		return oid
	}
	return oid[:8]
}
