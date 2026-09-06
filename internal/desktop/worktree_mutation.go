package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

// Create and move both name a destination the same way: a parent directory
// that already exists plus a single new directory name. Keeping them on one
// shape means the normalization and containment rules exist once.
type WorktreeLocation struct {
	ParentDirectory string `json:"parent_directory"`
	Name            string `json:"name"`
}

type CreateWorktreeRequest struct {
	Location  WorktreeLocation `json:"location"`
	Revision  string           `json:"revision"`
	NewBranch string           `json:"new_branch"`
	Detach    bool             `json:"detach"`
	Sync      string           `json:"sync"`
}

type MoveWorktreeRequest struct {
	Path     string           `json:"path"`
	Location WorktreeLocation `json:"location"`
}

type WorktreeMutationResult struct {
	State    RepositoryState `json:"state"`
	Path     string          `json:"path"`
	Warnings []string        `json:"warnings,omitempty"`
}

func (s *Service) CreateWorktree(ctx context.Context, request CreateWorktreeRequest) (WorktreeMutationResult, error) {
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}

	// syncBeforeAdd treats an empty mode as "ff-only", which fetches every
	// remote and fast-forwards the branch the user is currently looking at as
	// a side effect of creating a different worktree. Network work belongs to
	// the Commit screen, so only an explicit no-op is accepted here.
	switch strings.TrimSpace(request.Sync) {
	case "", "none":
	default:
		return WorktreeMutationResult{}, errors.New("remote sync is not available from the Worktree screen; use Sync on the Commit screen first")
	}

	destination, err := resolveWorktreeLocation(state, request.Location)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	if err := worktreeDestinationIsFree(state, destination); err != nil {
		return WorktreeMutationResult{}, err
	}

	newBranch := strings.TrimSpace(request.NewBranch)
	revision := strings.TrimSpace(request.Revision)
	if newBranch != "" && request.Detach {
		return WorktreeMutationResult{}, errors.New("a new branch and a detached checkout cannot be combined")
	}
	if newBranch != "" {
		// check-ref-format does not accept --end-of-options, so a dashed name
		// would be read as a flag and produce a usage error instead of an answer.
		if strings.HasPrefix(newBranch, "-") {
			return WorktreeMutationResult{}, fmt.Errorf("%q is not a valid branch name", newBranch)
		}
		if _, refErr := repository.Run(operationContext, nil, "check-ref-format", "--branch", newBranch); refErr != nil {
			return WorktreeMutationResult{}, fmt.Errorf("%q is not a valid branch name", newBranch)
		}
		if branchExists(operationContext, repository, newBranch) {
			return WorktreeMutationResult{}, fmt.Errorf("branch %s already exists; check it out instead of creating it", newBranch)
		}
	}
	if newBranch == "" && !request.Detach {
		if revision == "" {
			return WorktreeMutationResult{}, errors.New("choose a branch or start point, create a new branch, or check out detached")
		}
		if holder := worktreeHoldingBranch(state, revision); holder != "" {
			return WorktreeMutationResult{}, fmt.Errorf("branch %s is already checked out in %s", revision, holder)
		}
	}
	revisionHead := ""
	if revision != "" {
		if revisionHead, err = resolveCommit(operationContext, repository, revision); err != nil {
			return WorktreeMutationResult{}, fmt.Errorf("resolve start point %s: %w", revision, err)
		}
	}

	// Re-verify against live Git rather than the snapshot: the destination may
	// have appeared and the start point may have moved while the dialog was open.
	if err := worktreeDestinationIsFree(state, destination); err != nil {
		return WorktreeMutationResult{}, err
	}
	if newBranch != "" && branchExists(operationContext, repository, newBranch) {
		return WorktreeMutationResult{}, fmt.Errorf("branch %s appeared while the worktree was being prepared", newBranch)
	}
	if revision != "" {
		current, resolveErr := resolveCommit(operationContext, repository, revision)
		if resolveErr != nil || current != revisionHead {
			return WorktreeMutationResult{}, fmt.Errorf("start point %s changed while the worktree was being prepared", revision)
		}
	}

	mutation, err := app.NewWorktreeService(repository).Add(operationContext, app.AddWorktreeOptions{
		Path:      destination,
		Revision:  revision,
		NewBranch: newBranch,
		Detach:    request.Detach,
		Sync:      "none",
	})
	if err != nil {
		return WorktreeMutationResult{}, worktreeMutationError("create worktree", err)
	}

	next, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	return WorktreeMutationResult{State: next, Path: mutation.Path, Warnings: mutation.Warnings}, nil
}

func (s *Service) MoveWorktree(ctx context.Context, request MoveWorktreeRequest) (WorktreeMutationResult, error) {
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}

	source, target, err := registeredWorktree(state, request.Path)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	if source == canonicalWorktreePath(state.ProjectRoot) {
		return WorktreeMutationResult{}, errors.New("the main worktree cannot be moved")
	}
	// The Service pins its repository handle to the viewed root, so moving it
	// would leave every later command pointing at a path that no longer exists.
	if source == canonicalWorktreePath(state.Root) {
		return WorktreeMutationResult{}, errors.New("the currently viewed worktree cannot be moved; switch to Main first")
	}
	if target.Locked {
		return WorktreeMutationResult{}, fmt.Errorf("unlock worktree %s before moving it", worktreeLabel(target))
	}
	if target.Prunable {
		return WorktreeMutationResult{}, fmt.Errorf("worktree %s is missing from disk; prune it instead", worktreeLabel(target))
	}
	if target.Dirty {
		return WorktreeMutationResult{}, fmt.Errorf("worktree %s has local changes", worktreeLabel(target))
	}

	destination, err := resolveWorktreeLocation(state, request.Location)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	if err := worktreeDestinationIsFree(state, destination); err != nil {
		return WorktreeMutationResult{}, err
	}
	status, statusErr := repository.Runner.Run(operationContext, target.Path, nil, "status", "--porcelain=v2", "-z")
	if statusErr != nil {
		return WorktreeMutationResult{}, fmt.Errorf("verify worktree %s status: %w", worktreeLabel(target), statusErr)
	}
	if len(status) > 0 {
		return WorktreeMutationResult{}, fmt.Errorf("worktree %s changed while the move was being prepared", worktreeLabel(target))
	}
	if _, err := os.Lstat(destination); err == nil {
		return WorktreeMutationResult{}, fmt.Errorf("a file or directory appeared at %s while the move was being prepared", destination)
	}

	if _, err := app.NewWorktreeService(repository).Move(operationContext, target.Path, destination); err != nil {
		return WorktreeMutationResult{}, worktreeMutationError("move worktree", err)
	}

	next, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	return WorktreeMutationResult{State: next, Path: canonicalWorktreePath(destination)}, nil
}

// A destination is described by an existing parent plus one new segment.
// EvalSymlinks cannot resolve a path that does not exist yet, so the parent is
// canonicalized and the name joined onto it; otherwise /var and /private/var
// would compare unequal and the containment checks below would let a
// destination inside a registered worktree through.
func resolveWorktreeLocation(state RepositoryState, location WorktreeLocation) (string, error) {
	name := strings.TrimSpace(location.Name)
	if name == "" {
		return "", errors.New("a worktree name is required")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", errors.New("the worktree name cannot contain a path separator")
	}
	if strings.ContainsAny(name, "\x00\r\n") {
		return "", errors.New("the worktree name cannot contain a newline or NUL byte")
	}
	if name == "." || name == ".." || strings.HasPrefix(name, "-") {
		return "", fmt.Errorf("%q is not a usable worktree name", name)
	}

	parent := strings.TrimSpace(location.ParentDirectory)
	if parent == "" {
		parent = filepath.Dir(canonicalWorktreePath(state.ProjectRoot))
	}
	if !filepath.IsAbs(parent) {
		return "", errors.New("the parent directory must be an absolute path")
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("worktree parent directory does not exist: %s", parent)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("worktree parent is not a directory: %s", parent)
	}
	return filepath.Join(canonicalWorktreePath(parent), name), nil
}

func worktreeDestinationIsFree(state RepositoryState, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("a file or directory already exists at %s", destination)
	}
	for _, worktree := range state.Worktrees {
		registered := canonicalWorktreePath(worktree.Path)
		if registered == destination {
			return fmt.Errorf("%s is already a registered worktree", destination)
		}
		if pathContains(registered, destination) {
			return fmt.Errorf("%s is inside the existing worktree %s", destination, registered)
		}
		if pathContains(destination, registered) {
			return fmt.Errorf("%s would contain the existing worktree %s", destination, registered)
		}
	}
	return nil
}

func registeredWorktree(state RepositoryState, path string) (string, app.WorktreeInfo, error) {
	if strings.TrimSpace(path) == "" {
		return "", app.WorktreeInfo{}, errors.New("select a worktree first")
	}
	canonical := canonicalWorktreePath(path)
	for _, worktree := range state.Worktrees {
		if canonicalWorktreePath(worktree.Path) == canonical {
			return canonical, worktree, nil
		}
	}
	return "", app.WorktreeInfo{}, fmt.Errorf("worktree is not registered: %s", path)
}

func pathContains(parent, child string) bool {
	if parent == child {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func branchExists(ctx context.Context, repository *gitexec.Repository, branch string) bool {
	_, err := repository.Run(ctx, nil, "show-ref", "--verify", "--quiet", "--end-of-options", "refs/heads/"+branch)
	return err == nil
}

func worktreeHoldingBranch(state RepositoryState, branch string) string {
	for _, worktree := range state.Worktrees {
		if !worktree.Detached && worktree.Branch == branch {
			return worktree.Path
		}
	}
	return ""
}

func worktreeLabel(worktree app.WorktreeInfo) string {
	if worktree.Branch != "" {
		return worktree.Branch
	}
	return worktree.Path
}

// Wails marshals an error as its message alone, so the structured apperr code
// and details have to become part of the sentence here or they are lost. Only
// dirty_paths_outside_sparse carries detail the user cannot otherwise act on:
// it must name the paths that block the change.
func worktreeMutationError(action string, err error) error {
	code, _, details := apperr.Details(err)
	if code == "dirty_paths_outside_sparse" {
		if paths := detailStrings(details["paths"]); len(paths) > 0 {
			listed := paths
			suffix := ""
			if len(listed) > 5 {
				suffix = fmt.Sprintf(" (and %d more)", len(listed)-5)
				listed = listed[:5]
			}
			return fmt.Errorf(
				"%s: these paths have local changes outside the new selection and would be hidden: %s%s; commit or stash them, or include their directories",
				action, strings.Join(listed, ", "), suffix,
			)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}

func detailStrings(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

type sparseMutation struct {
	action      string
	directories []string
}

func (s *Service) SetWorktreeSparseDirectories(ctx context.Context, path string, directories []string) (WorktreeMutationResult, error) {
	return s.mutateWorktreeSparse(ctx, path, sparseMutation{action: "set", directories: directories})
}

func (s *Service) ExpandWorktreeSparseDirectories(ctx context.Context, path string, directories []string) (WorktreeMutationResult, error) {
	return s.mutateWorktreeSparse(ctx, path, sparseMutation{action: "expand", directories: directories})
}

func (s *Service) ContractWorktreeSparseDirectories(ctx context.Context, path string, directories []string) (WorktreeMutationResult, error) {
	return s.mutateWorktreeSparse(ctx, path, sparseMutation{action: "contract", directories: directories})
}

func (s *Service) DisableWorktreeSparseCheckout(ctx context.Context, path string) (WorktreeMutationResult, error) {
	return s.mutateWorktreeSparse(ctx, path, sparseMutation{action: "disable"})
}

// All four sparse operations share one guard list so the preconditions cannot
// drift apart between them.
func (s *Service) mutateWorktreeSparse(ctx context.Context, path string, mutation sparseMutation) (WorktreeMutationResult, error) {
	s.rewriteMu.Lock()
	defer s.rewriteMu.Unlock()
	operationContext, finish := s.beginOperation(ctx)
	defer finish()
	repository, err := s.currentRepository()
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	state, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}

	canonical, target, err := registeredWorktree(state, path)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	if target.Bare {
		return WorktreeMutationResult{}, errors.New("a bare worktree has no sparse-checkout to manage")
	}
	if target.Locked {
		return WorktreeMutationResult{}, fmt.Errorf("unlock worktree %s before changing its sparse-checkout", worktreeLabel(target))
	}
	if info, statErr := os.Stat(canonical); statErr != nil || !info.IsDir() {
		return WorktreeMutationResult{}, fmt.Errorf("worktree directory is missing: %s", canonical)
	}
	// Pre-empt the domain's non_cone_unsupported so the UI can disable the
	// control with a reason instead of failing on submit.
	if target.Sparse.Enabled && !target.Sparse.Cone {
		return WorktreeMutationResult{}, fmt.Errorf("worktree %s uses a non-cone sparse checkout; GitGit only manages cone mode", worktreeLabel(target))
	}
	if target.Head == "" {
		return WorktreeMutationResult{}, fmt.Errorf("worktree %s has an unborn HEAD", worktreeLabel(target))
	}
	if mutation.action == "disable" && !target.Sparse.Enabled {
		return WorktreeMutationResult{}, fmt.Errorf("sparse-checkout is not enabled for %s", worktreeLabel(target))
	}
	if mutation.action != "disable" && len(mutation.directories) == 0 {
		return WorktreeMutationResult{}, errors.New("select at least one directory")
	}

	worktreeService := app.NewWorktreeService(repository)
	// Only directories being added need to exist in this worktree's HEAD. The
	// current selection may legitimately name directories that live on another
	// branch, and contract or a mixed set resends the whole result set.
	for _, directory := range addedSparseDirectories(target.Sparse, mutation) {
		if err := verifyTreeDirectory(operationContext, repository, target.Head, directory); err != nil {
			return WorktreeMutationResult{}, err
		}
	}
	// SparseSet only probes for dirty paths when sparse checkout is already on,
	// so the first enable would otherwise hide local changes without warning.
	if mutation.action == "set" && !target.Sparse.Enabled {
		if err := worktreeService.CheckSparseRules(operationContext, canonical, mutation.directories); err != nil {
			return WorktreeMutationResult{}, worktreeMutationError("update sparse-checkout", err)
		}
	}

	current, err := worktreeService.SparseList(operationContext, canonical)
	if err != nil {
		return WorktreeMutationResult{}, fmt.Errorf("read sparse-checkout for %s: %w", worktreeLabel(target), err)
	}
	if current.Enabled != target.Sparse.Enabled || current.Cone != target.Sparse.Cone ||
		strings.Join(current.Directories, "\x00") != strings.Join(target.Sparse.Directories, "\x00") {
		return WorktreeMutationResult{}, fmt.Errorf("sparse-checkout for %s changed while the update was being prepared", worktreeLabel(target))
	}

	switch mutation.action {
	case "set":
		_, err = worktreeService.SparseSet(operationContext, canonical, mutation.directories)
	case "expand":
		_, err = worktreeService.SparseExpand(operationContext, canonical, mutation.directories)
	case "contract":
		_, err = worktreeService.SparseContract(operationContext, canonical, mutation.directories)
	case "disable":
		_, err = worktreeService.SparseDisable(operationContext, canonical)
	default:
		return WorktreeMutationResult{}, fmt.Errorf("unsupported sparse action %q", mutation.action)
	}
	if err != nil {
		return WorktreeMutationResult{}, worktreeMutationError("update sparse-checkout", err)
	}

	next, err := s.snapshot(operationContext, repository)
	if err != nil {
		return WorktreeMutationResult{}, err
	}
	return WorktreeMutationResult{State: next, Path: canonical}, nil
}

func addedSparseDirectories(current app.SparseState, mutation sparseMutation) []string {
	if mutation.action == "disable" || mutation.action == "contract" {
		return nil
	}
	existing := make(map[string]struct{}, len(current.Directories))
	for _, directory := range current.Directories {
		existing[directory] = struct{}{}
	}
	added := make([]string, 0, len(mutation.directories))
	for _, directory := range mutation.directories {
		if _, known := existing[directory]; !known {
			added = append(added, directory)
		}
	}
	return added
}

func verifyTreeDirectory(ctx context.Context, repository *gitexec.Repository, head, directory string) error {
	if strings.ContainsAny(directory, "\x00\r\n") || strings.HasPrefix(directory, "-") {
		return fmt.Errorf("%q is not a usable directory", directory)
	}
	out, err := repository.Run(ctx, nil, "cat-file", "-t", "--end-of-options", head+":"+directory)
	if err != nil || strings.TrimSpace(string(out)) != "tree" {
		return fmt.Errorf("%s is not a directory in this worktree", directory)
	}
	return nil
}
