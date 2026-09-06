package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestWorktreeAddSyncsByFastForward(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	gitTestCommand(t, base, "init", "--bare", origin)

	seed := filepath.Join(base, "seed")
	gitTestCommand(t, base, "init", "-b", "main", seed)
	gitTestCommand(t, seed, "config", "user.name", "Test User")
	gitTestCommand(t, seed, "config", "user.email", "test@example.com")
	writeTestFile(t, seed, "README.md", "first\n")
	gitTestCommand(t, seed, "add", ".")
	gitTestCommand(t, seed, "commit", "-m", "first")
	gitTestCommand(t, seed, "remote", "add", "origin", origin)
	gitTestCommand(t, seed, "push", "-u", "origin", "main")

	source := filepath.Join(base, "source")
	gitTestCommand(t, base, "clone", "-b", "main", origin, source)
	gitTestCommand(t, source, "config", "user.name", "Test User")
	gitTestCommand(t, source, "config", "user.email", "test@example.com")

	writeTestFile(t, seed, "README.md", "second\n")
	gitTestCommand(t, seed, "add", ".")
	gitTestCommand(t, seed, "commit", "-m", "second")
	gitTestCommand(t, seed, "push", "origin", "main")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), source)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)
	linked := filepath.Join(base, "feature")
	result, err := service.Add(context.Background(), AddWorktreeOptions{
		Path: linked, NewBranch: "feature", Sync: "ff-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != linked {
		t.Fatalf("unexpected linked path %q", result.Path)
	}
	content, err := os.ReadFile(filepath.Join(linked, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "second\n" {
		t.Fatalf("worktree was not created from synchronized HEAD: %q", content)
	}
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two worktrees, got %#v", items)
	}
}

func TestSparseExpandContractProtectsDirtyPaths(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "a/a.txt", "a\n")
	writeTestFile(t, root, "b/b.txt", "b\n")
	writeTestFile(t, root, "root.txt", "root\n")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "commit", "-m", "tree")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)
	state, err := service.SparseSet(context.Background(), root, []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || strings.Join(state.Directories, ",") != "a" {
		t.Fatalf("unexpected sparse state: %#v", state)
	}
	if _, err := os.Stat(filepath.Join(root, "b/b.txt")); !os.IsNotExist(err) {
		t.Fatalf("b should not be materialized, stat error: %v", err)
	}

	state, err = service.SparseExpand(context.Background(), root, []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(state.Directories, ",") != "a,b" {
		t.Fatalf("unexpected expanded state: %#v", state)
	}
	writeTestFile(t, root, "b/local.txt", "do not hide\n")
	if _, err := service.SparseContract(context.Background(), root, []string{"b"}); err == nil {
		t.Fatal("expected dirty sparse contraction to fail")
	}
	if _, err := os.Stat(filepath.Join(root, "b/local.txt")); err != nil {
		t.Fatalf("dirty file was lost: %v", err)
	}
}

// SparseSet skips protectDirtyPaths when sparse checkout is still disabled, so
// the very first selection is the one transition that can hide a dirty file
// without any warning. CheckSparseRules exists to close that gap.
func TestCheckSparseRulesRejectsDirtyPathsBeforeSparseIsEnabled(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "a/a.txt", "a\n")
	writeTestFile(t, root, "b/b.txt", "b\n")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "commit", "-m", "tree")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)

	writeTestFile(t, root, "b/local.txt", "do not hide\n")
	if err := service.CheckSparseRules(context.Background(), root, []string{"a"}); err == nil {
		t.Fatal("expected a dirty path outside the new selection to be rejected")
	} else if code := appErrorCode(err); code != "dirty_paths_outside_sparse" {
		t.Fatalf("error code = %q, want %q", code, "dirty_paths_outside_sparse")
	}

	// The probe must not touch the checkout, and a selection that keeps the
	// dirty path visible stays allowed.
	if _, statErr := os.Stat(filepath.Join(root, "b/local.txt")); statErr != nil {
		t.Fatalf("dirty file was lost by the probe: %v", statErr)
	}
	if err := service.CheckSparseRules(context.Background(), root, []string{"a", "b"}); err != nil {
		t.Fatalf("selection covering the dirty path should be allowed: %v", err)
	}
}
