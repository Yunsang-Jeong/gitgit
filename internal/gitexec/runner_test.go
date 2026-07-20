package gitexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerAndRepositoryLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runner := NewRunner()
	if runner.Binary != "git" {
		t.Fatalf("default binary = %q, want git", runner.Binary)
	}
	version, err := runner.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, "git version ") {
		t.Fatalf("unexpected Git version: %q", version)
	}

	base := t.TempDir()
	root := filepath.Join(base, "repository")
	if _, err := runner.Run(ctx, base, nil, "init", "-b", "main", root); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := OpenRepository(ctx, runner, nested)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Root != resolvedRoot {
		t.Fatalf("repository root = %q, want %q", repo.Root, resolvedRoot)
	}

	status, err := repo.Run(ctx, nil, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 0 {
		t.Fatalf("new repository is unexpectedly dirty: %q", status)
	}
	hash, err := runner.Run(ctx, root, strings.NewReader("randomized fixture\n"), "hash-object", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(hash)); len(got) != 40 {
		t.Fatalf("hash-object returned %q", got)
	}
}

func TestRunnerIgnoresAmbientRepositorySelectionEnvironment(t *testing.T) {
	ctx := context.Background()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	wanted := filepath.Join(root, "wanted")
	ambient := filepath.Join(root, "ambient")
	for _, repository := range []string{wanted, ambient} {
		if output, initErr := exec.Command(realGit, "init", "-q", repository).CombinedOutput(); initErr != nil {
			t.Fatalf("init %s: %v: %s", repository, initErr, output)
		}
	}

	wrapper := filepath.Join(root, "git-wrapper")
	wrapperSource := `#!/bin/sh
test -z "$GIT_CONFIG_GLOBAL" || exit 93
test -z "$GIT_CONFIG_SYSTEM" || exit 94
test -z "$GIT_CONFIG_PARAMETERS" || exit 95
exec "$TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(wrapperSource), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REAL_GIT", realGit)
	t.Setenv("GIT_DIR", filepath.Join(ambient, ".git"))
	t.Setenv("GIT_WORK_TREE", ambient)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(ambient, ".git", "index"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "ambient-global-config"))
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(root, "ambient-system-config"))
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.bare'='true'")

	repository, err := OpenRepository(ctx, &Runner{Binary: wrapper}, wanted)
	if err != nil {
		t.Fatal(err)
	}
	wantedRoot, err := filepath.EvalSymlinks(wanted)
	if err != nil {
		t.Fatal(err)
	}
	if repository.Root != wantedRoot {
		t.Fatalf("repository root = %q, want %q", repository.Root, wantedRoot)
	}
}

func TestRunnerAndOpenRepositoryErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runner := NewRunner()
	if _, err := OpenRepository(ctx, runner, t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a Git worktree") {
		t.Fatalf("expected non-repository error, got %v", err)
	}

	_, err := runner.Run(ctx, t.TempDir(), nil, "definitely-not-a-git-command")
	if err == nil {
		t.Fatal("invalid Git command unexpectedly succeeded")
	}
	var commandErr *CommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("error type = %T, want *CommandError", err)
	}
	if len(commandErr.Args) == 0 || commandErr.Stderr == "" || commandErr.Unwrap() == nil {
		t.Fatalf("incomplete command error: %#v", commandErr)
	}
	if commandErr.Error() == "" {
		t.Fatal("command error has an empty message")
	}

	missing := &Runner{Binary: filepath.Join(t.TempDir(), "missing-git")}
	if _, err := missing.Version(ctx); err == nil {
		t.Fatal("missing Git binary unexpectedly succeeded")
	}
}
