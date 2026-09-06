package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openServiceAt(t *testing.T, root string) *Service {
	t.Helper()
	service := NewService(nil)
	t.Cleanup(func() { _ = service.Close() })
	if _, err := service.Open(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestCreateWorktreeValidatesDestinationWithoutTouchingTheRepository(t *testing.T) {
	root := createRepository(t)
	service := openServiceAt(t, root)
	parent := t.TempDir()
	occupied := filepath.Join(parent, "occupied")
	writeFile(t, filepath.Join(occupied, "keep.txt"), "keep\n")

	tests := []struct {
		name    string
		request CreateWorktreeRequest
		message string
	}{
		{
			name:    "missing name",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent}, NewBranch: "feature"},
			message: "worktree name is required",
		},
		{
			name:    "name with separator",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "nested/linked"}, NewBranch: "feature"},
			message: "cannot contain a path separator",
		},
		{
			name:    "parent traversal name",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: ".."}, NewBranch: "feature"},
			message: "not a usable worktree name",
		},
		{
			name:    "dashed name",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "--force"}, NewBranch: "feature"},
			message: "not a usable worktree name",
		},
		{
			name:    "relative parent",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: "relative", Name: "linked"}, NewBranch: "feature"},
			message: "must be an absolute path",
		},
		{
			name:    "missing parent",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: filepath.Join(parent, "absent"), Name: "linked"}, NewBranch: "feature"},
			message: "parent directory does not exist",
		},
		{
			name:    "occupied destination",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "occupied"}, NewBranch: "feature"},
			message: "already exists at",
		},
		{
			name:    "inside the main worktree",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: root, Name: "linked"}, NewBranch: "feature"},
			message: "is inside the existing worktree",
		},
		{
			name:    "branch and detach",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}, NewBranch: "feature", Detach: true},
			message: "cannot be combined",
		},
		{
			name:    "existing branch as new branch",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}, NewBranch: "main"},
			message: "already exists; check it out instead",
		},
		{
			name:    "branch already checked out",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}, Revision: "main"},
			message: "already checked out in",
		},
		{
			name:    "no start point",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}},
			message: "choose a branch or start point",
		},
		{
			name:    "unresolvable start point",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}, Revision: "does-not-exist"},
			message: "resolve start point",
		},
		{
			// ff-only fetches every remote and fast-forwards the viewed branch as
			// a side effect of creating a different worktree.
			name:    "remote sync mode",
			request: CreateWorktreeRequest{Location: WorktreeLocation{ParentDirectory: parent, Name: "linked"}, NewBranch: "feature", Sync: "ff-only"},
			message: "remote sync is not available",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := observeRepository(t, root)
			_, err := service.CreateWorktree(context.Background(), test.request)
			if err == nil {
				t.Fatal("expected the request to be rejected")
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %q, want it to contain %q", err, test.message)
			}
			if after := observeRepository(t, root); after != before {
				t.Fatal("a rejected request changed the repository")
			}
		})
	}
}

func TestCreateWorktreeAddsLinkedWorktreeWithoutFetching(t *testing.T) {
	root := createRepository(t)
	service := openServiceAt(t, root)
	parent := t.TempDir()

	result, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked worktree"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(result.Path); statErr != nil || !info.IsDir() {
		t.Fatalf("worktree directory was not created at %s: %v", result.Path, statErr)
	}
	found := false
	for _, worktree := range result.State.Worktrees {
		if canonicalWorktreePath(worktree.Path) == canonicalWorktreePath(result.Path) {
			found = true
			if worktree.Branch != "feature" {
				t.Fatalf("worktree branch = %q, want %q", worktree.Branch, "feature")
			}
		}
	}
	if !found {
		t.Fatal("the new worktree is missing from the returned state")
	}

	// Sync is forced to none, so no remote-tracking work may have happened.
	if refs := gitOutput(t, root, "for-each-ref", "--format=%(refname)", "refs/remotes"); strings.TrimSpace(refs) != "" {
		t.Fatalf("creating a worktree fetched remotes: %q", refs)
	}
}

func TestMoveWorktreeRefusesProtectedTargets(t *testing.T) {
	root := createRepository(t)
	service := openServiceAt(t, root)
	parent := t.TempDir()

	created, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(parent, "occupied")
	writeFile(t, filepath.Join(occupied, "keep.txt"), "keep\n")

	tests := []struct {
		name    string
		request MoveWorktreeRequest
		message string
	}{
		{
			name:    "unregistered source",
			request: MoveWorktreeRequest{Path: filepath.Join(parent, "absent"), Location: WorktreeLocation{ParentDirectory: parent, Name: "moved"}},
			message: "is not registered",
		},
		{
			name:    "main worktree",
			request: MoveWorktreeRequest{Path: root, Location: WorktreeLocation{ParentDirectory: parent, Name: "moved"}},
			message: "main worktree cannot be moved",
		},
		{
			name:    "occupied destination",
			request: MoveWorktreeRequest{Path: created.Path, Location: WorktreeLocation{ParentDirectory: parent, Name: "occupied"}},
			message: "already exists at",
		},
		{
			name:    "destination inside the source",
			request: MoveWorktreeRequest{Path: created.Path, Location: WorktreeLocation{ParentDirectory: created.Path, Name: "inner"}},
			// The source is itself a registered worktree, so the destination is
			// rejected as being inside it by name.
			message: "is inside the existing worktree",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := observeRepository(t, root)
			_, err := service.MoveWorktree(context.Background(), test.request)
			if err == nil {
				t.Fatal("expected the request to be rejected")
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %q, want it to contain %q", err, test.message)
			}
			if after := observeRepository(t, root); after != before {
				t.Fatal("a rejected request changed the repository")
			}
			if _, statErr := os.Stat(created.Path); statErr != nil {
				t.Fatalf("the source worktree was disturbed: %v", statErr)
			}
		})
	}
}

func TestMoveWorktreeRefusesDirtyAndRelocatesCleanWorktree(t *testing.T) {
	root := createRepository(t)
	service := openServiceAt(t, root)
	parent := t.TempDir()

	created, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(created.Path, "internal", "search.go"), "package internal\n\nconst engine = \"dirty\"\n")
	if _, err := service.MoveWorktree(context.Background(), MoveWorktreeRequest{
		Path:     created.Path,
		Location: WorktreeLocation{ParentDirectory: parent, Name: "moved"},
	}); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("expected a dirty worktree to be refused, got %v", err)
	}

	runGit(t, created.Path, nil, "checkout", "--", "internal/search.go")
	moved, err := service.MoveWorktree(context.Background(), MoveWorktreeRequest{
		Path:     created.Path,
		Location: WorktreeLocation{ParentDirectory: parent, Name: "moved"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(created.Path); !os.IsNotExist(statErr) {
		t.Fatalf("the old worktree path still exists: %v", statErr)
	}
	if info, statErr := os.Stat(moved.Path); statErr != nil || !info.IsDir() {
		t.Fatalf("the worktree is missing at %s: %v", moved.Path, statErr)
	}
	registered := gitOutput(t, root, "worktree", "list", "--porcelain")
	if strings.Contains(registered, created.Path) {
		t.Fatalf("the old path is still registered:\n%s", registered)
	}
}

func TestSparseMutationsFollowConeLifecycle(t *testing.T) {
	root := createRepository(t)
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# guide\n")
	writeFile(t, filepath.Join(root, "tools", "run.sh"), "echo run\n")
	runGit(t, root, nil, "add", "--all")
	runGit(t, root, nil, "commit", "-q", "-m", "feat: add docs and tools")

	service := openServiceAt(t, root)
	parent := t.TempDir()
	created, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetWorktreeSparseDirectories(context.Background(), created.Path, []string{"internal"}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(created.Path, "docs")); !os.IsNotExist(statErr) {
		t.Fatalf("docs should not be materialized: %v", statErr)
	}

	state, err := service.ExpandWorktreeSparseDirectories(context.Background(), created.Path, []string{"docs"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sparseDirectoriesOf(t, state, created.Path); got != "docs,internal" {
		t.Fatalf("expanded directories = %q, want %q", got, "docs,internal")
	}

	state, err = service.ContractWorktreeSparseDirectories(context.Background(), created.Path, []string{"docs"})
	if err != nil {
		t.Fatal(err)
	}
	if got := sparseDirectoriesOf(t, state, created.Path); got != "internal" {
		t.Fatalf("contracted directories = %q, want %q", got, "internal")
	}

	if _, err := service.DisableWorktreeSparseCheckout(context.Background(), created.Path); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(created.Path, "docs", "guide.md")); statErr != nil {
		t.Fatalf("disabling sparse-checkout did not restore docs: %v", statErr)
	}
	if _, err := service.DisableWorktreeSparseCheckout(context.Background(), created.Path); err == nil ||
		!strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected a second disable to be refused, got %v", err)
	}
}

// The message has to name the offending paths: worktreeMutationError is the
// only place the structured apperr detail survives the Wails boundary.
func TestSparseMutationNamesDirtyPathsItRefusesToHide(t *testing.T) {
	root := createRepository(t)
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# guide\n")
	runGit(t, root, nil, "add", "--all")
	runGit(t, root, nil, "commit", "-q", "-m", "feat: add docs")

	service := openServiceAt(t, root)
	parent := t.TempDir()
	created, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(created.Path, "docs", "local.md"), "do not hide\n")
	_, err = service.SetWorktreeSparseDirectories(context.Background(), created.Path, []string{"internal"})
	if err == nil {
		t.Fatal("expected the first enable to be refused over a dirty path")
	}
	if !strings.Contains(err.Error(), "docs/local.md") {
		t.Fatalf("error = %q, want it to name docs/local.md", err)
	}
	if _, statErr := os.Stat(filepath.Join(created.Path, "docs", "local.md")); statErr != nil {
		t.Fatalf("the dirty file was lost: %v", statErr)
	}
}

func TestSparseMutationRefusesDirectoryMissingFromWorktreeHead(t *testing.T) {
	root := createRepository(t)
	service := openServiceAt(t, root)
	parent := t.TempDir()
	created, err := service.CreateWorktree(context.Background(), CreateWorktreeRequest{
		Location:  WorktreeLocation{ParentDirectory: parent, Name: "linked"},
		NewBranch: "feature",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.SetWorktreeSparseDirectories(context.Background(), created.Path, []string{"intrenal"})
	if err == nil || !strings.Contains(err.Error(), "is not a directory in this worktree") {
		t.Fatalf("expected a typo to be refused, got %v", err)
	}
}

func sparseDirectoriesOf(t *testing.T, result WorktreeMutationResult, path string) string {
	t.Helper()
	for _, worktree := range result.State.Worktrees {
		if canonicalWorktreePath(worktree.Path) == canonicalWorktreePath(path) {
			return strings.Join(worktree.Sparse.Directories, ",")
		}
	}
	t.Fatalf("worktree %s is missing from the returned state", path)
	return ""
}
